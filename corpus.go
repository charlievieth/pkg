package pkg

import (
	"errors"
	"fmt"
	"go/build"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"time"
)

type Context struct {
	ctxt           *build.Context
	srcDirs        []string
	lastUpdate     time.Time
	updateInterval time.Duration
	mu             sync.RWMutex
}

func NewContext(ctxt *build.Context, updateInterval time.Duration) *Context {
	if updateInterval == 0 {
		updateInterval = time.Second
	}
	c := &Context{
		ctxt:           ctxt,
		updateInterval: updateInterval,
	}
	c.Update()
	return c
}

func (c *Context) Context() *build.Context {
	// WARN: We Copy-on-Write - so shouldnt need to lock
	// make sure this is actually the case.
	c.Update()
	return c.ctxt
}

func (c *Context) SrcDirs() []string {
	c.Update()
	return c.srcDirs
}

func (c *Context) GOROOT() string {
	c.Update()
	return c.ctxt.GOROOT
}

func (c *Context) MatchFile(dir, name string) bool {
	c.Update()
	ok, err := c.ctxt.MatchFile(dir, name)
	return ok && err == nil
}

func (c *Context) Update() {
	if c.outdated() {
		c.doUpdate()
	}
}

func (c *Context) outdated() bool {
	c.mu.RLock()
	update := time.Since(c.lastUpdate) < c.updateInterval
	c.mu.RUnlock()
	return update
}

func (c *Context) doUpdate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Make sure it wasnt updated before the lock was acquired
	if time.Since(c.lastUpdate) < c.updateInterval {
		return
	}
	if c.ctxt == nil {
		c.initDefault()
	}
	c.lastUpdate = time.Now()
	path := os.Getenv("GOPATH")
	root := runtime.GOROOT()
	switch {
	case path != c.ctxt.GOPATH || root != c.ctxt.GOROOT:
		// Copy and replace on change
		ctxt := *c.ctxt
		ctxt.GOPATH = path
		ctxt.GOROOT = root
		srcDirs := ctxt.SrcDirs()
		c.ctxt = &ctxt
		c.srcDirs = srcDirs

	case len(c.srcDirs) == 0:
		// In case we just initialized a new Context
		c.srcDirs = c.ctxt.SrcDirs()
	}
}

func (c *Context) initDefault() {
	ctxt := build.Default
	ctxt.GOPATH = os.Getenv("GOPATH")
	ctxt.GOROOT = runtime.GOROOT()
	c.ctxt = &ctxt
}

func newContext() (*build.Context, []string) {
	c := build.Default
	c.GOPATH = os.Getenv("GOPATH")
	c.GOROOT = runtime.GOROOT()
	return &c, c.SrcDirs()
}

type Corpus struct {
	ctxt       *Context
	lastUpdate time.Time

	dirs     map[string]*Directory
	srcDirs  []string
	MaxDepth int
	mu       sync.RWMutex

	// WARN: New
	IndexEnabled bool
	IndexGoCode  bool
}

// TODO: Do we care about missing GOROOT and GOPATH env vars?
func NewCorpus() *Corpus {
	dirs := make(map[string]*Directory)
	c := &Corpus{
		ctxt: NewContext(nil, 0),
		dirs: dirs,
		mu:   sync.RWMutex{},
	}
	fset := token.NewFileSet()
	t := newTreeBuilder(c)
	for _, path := range c.ctxt.SrcDirs() {
		dir := t.newDirTree(fset, path, filepath.Base(path), 0, false)
		if dir != nil {
			dirs[path] = dir
		}
	}
	return c
}

// TODO: Toggle 'internal' behavior based on Go version.
//
// ListImports, returns the list of available imports for path.
func (c *Corpus) ListImports(path string) []string {
	c.Update()
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.dirs == nil || len(c.dirs) == 0 {
		return nil // []string{} ???
	}
	list := make([]string, 0, 512)
	for _, d := range c.dirs {
		d.listPkgs(filepathDir(path), &list)
	}
	sort.Strings(list)
	return list
}

func (c *Corpus) Lookup(path string) *Directory {
	c.Update()
	c.mu.RLock()
	defer c.mu.RUnlock()
	for p, dir := range c.dirs {
		if filepath.HasPrefix(path, p) {
			if d := dir.Lookup(path); d != nil {
				return d
			}
		}
	}
	return nil
}

func (c *Corpus) Update() {
	c.mu.Lock()
	defer c.mu.Unlock()

	t := newTreeBuilder(c)
	seen := make(map[string]bool)
	for _, path := range c.srcDirs {
		seen[path] = true
		fset := token.NewFileSet()
		if _, ok := c.dirs[path]; ok {
			c.dirs[path] = t.updateDirTree(c.dirs[path], fset)
		} else {
			c.dirs[path] = t.newDirTree(fset, path, filepath.Base(path), 0, false)
		}
	}

	// Cleanup root directories
	for path := range seen {
		if !seen[path] {
			delete(c.dirs, path)
		}
	}
}

func (c *Corpus) SrcDirs() (s []string) {
	c.mu.RLock()
	s = c.srcDirs
	c.mu.RUnlock()
	return
}

// WARN: Do we want to remove directories?

func (c *Corpus) matchFile(dir, name string) (match bool) {
	if ctxt := c.ctxt.Context(); ctxt != nil {
		ok, err := c.ctxt.Context().MatchFile(dir, name)
		match = (ok && err == nil)
	}
	return
}

type File struct {
	Name string
	Path string
	Info os.FileInfo
}

func NewFile(path string) (*File, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if fi.IsDir() {
		return nil, errors.New("pkg: directory path: " + path)
	}
	return &File{
		Name: filepath.Base(path),
		Path: path,
		Info: fi,
	}, nil
}

type ImportMode int

const (
	// If FindPackage is set, NewPackage stops after reading ony package
	// statement.
	FindPackage ImportMode = 1 << iota

	// If CheckPackage is set, NewPackage checks all package statements.
	CheckPackage

	// TODO: Implement
	// If IndexPackage is set, Package decls are indexed
	IndexPackage
)

type Package struct {
	Dir        string // directory path
	Name       string // package name
	ImportPath string // import path of package ("" if unknown)
	Root       string // root of Go tree where this package lives
	Goroot     bool   // package found in Go root

	GoFiles        []string // .go source files (excluding TestGoFiles, XTestGoFiles)
	IgnoredGoFiles []string // .go source files ignored for this build
	TestGoFiles    []string // _test.go files in package

	mode ImportMode // ImportMode used when created
}

// IsCommand reports whether the package is considered a command to be installed
// (not just a library). Packages named "main" are treated as commands.
func (p *Package) IsCommand() bool {
	return p.Name == "main"
}

func (c *Corpus) updatePackage(p *Package) {

}

func (c *Corpus) NewPackage(dir string, mode ImportMode) *Package {
	p, _ := c.importPackage(dir, token.NewFileSet(), mode)
	return p
}

func (c *Corpus) importPackage(dir string, fset *token.FileSet, mode ImportMode) (*Package, error) {
	names, err := readdirnames(dir)
	if err != nil {
		return nil, err
	}
	p := Package{
		Dir:  dir,
		mode: mode,
	}
	// SrcDirs returns $GOPATH + "/src"
	for _, srcDir := range c.srcDirs {
		if sameRoot(dir, srcDir) {
			p.ImportPath = trimPathPrefix(dir, srcDir)
			p.Root = filepath.Dir(srcDir)
			p.Goroot = sameRoot(dir, c.ctxt.GOROOT())
			break
		}
	}
	for _, name := range FilterList(names, isGoFile) {
		if err := p.addFile(c, fset, name); err != nil {
			return &p, err
		}
	}
	var pkgErr error
	if p.Name == "" {
		pkgErr = &NoGoError{Dir: dir}
	}
	return &p, pkgErr
}

func (p *Package) ignoreFile(c *Corpus, name string) bool {
	if !isGoFile(name) {
		return true
	}
	if !c.matchFile(p.Dir, name) {
		p.IgnoredGoFiles = append(p.IgnoredGoFiles, name)
		return true
	}
	if isGoTestFile(name) {
		p.TestGoFiles = append(p.TestGoFiles, name)
		return true
	}
	return false
}

func (p *Package) visitFile(c *Corpus, fset *token.FileSet, name string) error {
	switch {
	case !isGoFile(name):
		// Skip
	case !c.matchFile(p.Dir, name):
		p.IgnoredGoFiles = append(p.IgnoredGoFiles, name)
	case isGoTestFile(name):
		p.TestGoFiles = append(p.TestGoFiles, name)
	default:
		// fi, err := os.Stat(name)

	}
	switch {
	case p.mode&CheckPackage != 0:
		n, ok := parseFileName(filepath.Join(p.Dir, name), fset)
		if !ok {
			return nil
		}
		switch {
		case p.Name == "":
			p.Name = n
		case p.Name != n:
			return &MultiplePackageError{
				Dir:      p.Dir,
				Packages: []string{p.Name, n},
				Files:    []string{p.GoFiles[0], name},
			}
		}
	case p.Name == "":
		n, ok := parsePkgName(filepath.Join(p.Dir, name), fset)
		if ok {
			p.Name = n
		}
	}
	return nil
}

func (p *Package) XX_addFile(c *Corpus, fset *token.FileSet, name string, fi os.FileInfo) error {
	p.GoFiles = append(p.GoFiles, name)
	switch {
	case p.mode&CheckPackage != 0:
		n, ok := parseFileName(filepath.Join(p.Dir, name), fset)
		if !ok {
			return nil
		}
		switch {
		case p.Name == "":
			p.Name = n
		case p.Name != n:
			return &MultiplePackageError{
				Dir:      p.Dir,
				Packages: []string{p.Name, n},
				Files:    []string{p.GoFiles[0], name},
			}
		}
	case p.Name == "":
		n, ok := parsePkgName(filepath.Join(p.Dir, name), fset)
		if ok {
			p.Name = n
		}
	}
	return nil
}

func (p *Package) addFile(c *Corpus, fset *token.FileSet, name string) error {
	if !isGoFile(name) {
		return nil
	}
	if !c.matchFile(p.Dir, name) {
		p.IgnoredGoFiles = append(p.IgnoredGoFiles, name)
		return nil
	}
	if isGoTestFile(name) {
		p.TestGoFiles = append(p.TestGoFiles, name)
		return nil
	}
	p.GoFiles = append(p.GoFiles, name)
	switch {
	case p.mode&CheckPackage != 0:
		n, ok := parseFileName(filepath.Join(p.Dir, name), fset)
		if !ok {
			return nil
		}
		switch {
		case p.Name == "":
			p.Name = n
		case p.Name != n:
			return &MultiplePackageError{
				Dir:      p.Dir,
				Packages: []string{p.Name, n},
				Files:    []string{p.GoFiles[0], name},
			}
		}
	case p.Name == "":
		n, ok := parsePkgName(filepath.Join(p.Dir, name), fset)
		if ok {
			p.Name = n
		}
	}
	return nil
}

// NoGoError is the error used by Import to describe a directory
// containing no buildable Go source files. (It may still contain
// test files, files hidden by build tags, and so on.)
type NoGoError struct {
	Dir string
}

func (e *NoGoError) Error() string {
	return "no buildable Go source files in " + e.Dir
}

// MultiplePackageError describes a directory containing
// multiple buildable Go source files for multiple packages.
type MultiplePackageError struct {
	Dir      string   // directory containing files
	Packages []string // package names found
	Files    []string // corresponding files: Files[i] declares package Packages[i]
}

func (e *MultiplePackageError) Error() string {
	// Error string limited to two entries for compatibility.
	return fmt.Sprintf("found packages %s (%s) and %s (%s) in %s", e.Packages[0],
		e.Files[0], e.Packages[1], e.Files[1], e.Dir)
}
