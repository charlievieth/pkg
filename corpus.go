package pkg

import (
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

const maxOpenFiles = 200

type Corpus struct {
	ctxt       *Context
	lastUpdate time.Time

	dirs     map[string]*Directory
	srcDirs  []string
	MaxDepth int
	mu       sync.RWMutex

	PackageMode   ImportMode
	IndexFileInfo bool
	// WARN: New
	IndexEnabled bool
	IndexGoCode  bool

	fsOpenGate chan bool
}

// TODO: Do we care about missing GOROOT and GOPATH env vars?
func NewCorpus() *Corpus {
	dirs := make(map[string]*Directory)
	c := &Corpus{
		ctxt:       NewContext(nil, 0),
		dirs:       dirs,
		mu:         sync.RWMutex{},
		fsOpenGate: make(chan bool, maxOpenFiles),
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
	Name string      // file name
	Path string      // absolute file path
	Info os.FileInfo // info, if any
}

func NewFile(path string, info bool) (*File, error) {
	f := &File{
		Name: filepath.Base(path),
		Path: path,
	}
	if info {
		fi, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		if fi.IsDir() {
			return nil, errors.New("pkg: directory path: " + path)
		}
	}
	return f, nil
}

type ByFileName []*File

func (f ByFileName) Len() int           { return len(f) }
func (f ByFileName) Less(i, j int) bool { return f[i].Name < f[j].Name }
func (f ByFileName) Swap(i, j int)      { f[i].Name, f[j].Name = f[j].Name, f[i].Name }

type ImportMode int

const (
	// If FindPackageOnly is set, NewPackage stops after reading ony package
	// statement.
	FindPackageOnly ImportMode = 1 << iota

	// If FindPackageName is set,
	FindPackageName

	// If IndexPackage is set, Package files are indexed
	FindPackageFiles
)

type FileMap map[string]*File

func (m FileMap) Files() []*File {
	fs := make([]*File, 0, len(m))
	for _, f := range m {
		fs = append(fs, f)
	}
	sort.Sort(ByFileName(fs))
	return fs
}

type Package struct {
	Dir        string // directory path
	Name       string // package name
	ImportPath string // import path of package ("" if unknown)
	Root       string // root of Go tree where this package lives
	Goroot     bool   // package found in Go root

	// GoFiles        map[string]*File // .go source files (excluding TestGoFiles, XTestGoFiles)
	// IgnoredGoFiles map[string]*File // .go source files ignored for this build
	// TestGoFiles    map[string]*File // _test.go files in package

	GoFiles        FileMap // .go source files (excluding TestGoFiles, XTestGoFiles)
	IgnoredGoFiles FileMap // .go source files ignored for this build
	TestGoFiles    FileMap // _test.go files in package

	Info os.FileInfo

	mode ImportMode // ImportMode used when created
}

func (p *Package) FindPackageOnly() bool {
	return p.mode&FindPackageOnly != 0
}

func (p *Package) FindPackageName() bool {
	return p.mode&FindPackageName != 0
}

func (p *Package) FindPackageFiles() bool {
	return p.mode&FindPackageFiles != 0
}

func (p *Package) LookupFile(name string) (*File, bool) {
	if f, ok := p.GoFiles[name]; ok {
		return f, ok
	}
	if f, ok := p.IgnoredGoFiles[name]; ok {
		return f, ok
	}
	if f, ok := p.TestGoFiles[name]; ok {
		return f, ok
	}
	return nil, false
}

func (p *Package) DeleteFile(name string) {
	delete(p.GoFiles, name)
	delete(p.IgnoredGoFiles, name)
	delete(p.TestGoFiles, name)
}

func (p *Package) hasFiles() bool {
	return len(p.GoFiles) != 0 && len(p.IgnoredGoFiles) != 0 &&
		len(p.TestGoFiles) != 0
}

// IsCommand reports whether the package is considered a command to be installed
// (not just a library). Packages named "main" are treated as commands.
func (p *Package) IsCommand() bool {
	return p.Name == "main"
}

func (c *Corpus) NewPackage(dir string, mode ImportMode) *Package {
	p, _ := c.importPackage(dir, nil, token.NewFileSet(), nil)
	return p
}

// TODO: Organize args
func (c *Corpus) importPackage(dir string, fi os.FileInfo, fset *token.FileSet,
	names []string) (*Package, error) {

	p := &Package{
		Dir:            dir,
		mode:           c.PackageMode,
		Info:           fi,
		GoFiles:        make(FileMap),
		IgnoredGoFiles: make(FileMap),
		TestGoFiles:    make(FileMap),
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
	var pkgErr error
	if len(names) != 0 || p.mode > FindPackageOnly {
		var err error
		if len(names) == 0 {
			names, err = readdirnames(dir)
			if err != nil {
				return nil, err
			}
		}
		for _, name := range names {
			if err := c.addFile(p, name, fset); err != nil {
				pkgErr = err
			}
		}
		if p.Name == "" {
			pkgErr = &NoGoError{Dir: dir}
		}
	}
	if p.hasFiles() {
		return p, pkgErr
	}
	return nil, pkgErr
}

var ErrPackageNotExist = errors.New("pkg: package directory does not exists")

// TODO: Organize args
func (c *Corpus) updatePackage(p *Package, fi os.FileInfo, fset *token.FileSet,
	names []string) (*Package, error) {

	if !fi.IsDir() {
		return nil, ErrPackageNotExist
	}
	var pkgErr error
	p.mode = c.PackageMode
	if len(names) != 0 || p.mode > FindPackageOnly {
		var err error
		if len(names) == 0 {
			names, err = readdirnames(p.Dir)
			if err != nil {
				return nil, err
			}
		}
		seen := make(map[string]bool, len(names))
		for _, name := range names {
			if err := c.updateFile(p, name, fset); err != nil {
				pkgErr = err
			}
		}
		for name := range p.GoFiles {
			if !seen[name] {
				delete(p.GoFiles, name)
			}
		}
		for name := range p.TestGoFiles {
			if !seen[name] {
				delete(p.TestGoFiles, name)
			}
		}
		for name := range p.IgnoredGoFiles {
			if !seen[name] {
				delete(p.IgnoredGoFiles, name)
			}
		}
	}
	if p.hasFiles() {
		return p, pkgErr
	}
	return nil, pkgErr
}

func (c *Corpus) updateFile(p *Package, name string, fset *token.FileSet) error {
	f, ok := p.LookupFile(name)
	if !ok {
		return c.addFile(p, name, fset)
	}
	if c.IndexFileInfo {
		fi, err := os.Stat(f.Path)
		if err != nil {
			p.DeleteFile(name)
			return err
		}
		if sameFile(f.Info, fi) {
			return nil
		}
		if fi.IsDir() {
			p.DeleteFile(name)
			return nil
		}
		f.Info = fi
	}
	if isGoTestFile(name) {
		return nil
	}
	index := false
	if c.ctxt.MatchFile(p.Dir, name) {
		if _, ok := p.IgnoredGoFiles[name]; ok {
			delete(p.IgnoredGoFiles, name)
			p.GoFiles[name] = f
		}
		index = true
	} else {
		if _, ok := p.GoFiles[name]; ok {
			delete(p.GoFiles, name)
			p.IgnoredGoFiles[name] = f
		}
	}
	if index && p.mode > FindPackageOnly {
		return c.indexFile(p, f, fset)
	}
	return nil
}

func (c *Corpus) addFile(p *Package, name string, fset *token.FileSet) error {
	if !isGoFile(name) {
		return nil
	}
	path := filepath.Join(p.Dir, name)
	f, err := NewFile(path, c.IndexFileInfo)
	if err != nil {
		return err
	}
	index := false
	switch {
	case isGoTestFile(name):
		p.TestGoFiles[name] = f
	case c.ctxt.MatchFile(p.Dir, name):
		p.GoFiles[name] = f
		index = true
	default:
		p.IgnoredGoFiles[name] = f
	}
	if index && p.mode > FindPackageOnly {
		return c.indexFile(p, f, fset)
	}
	return nil
}

// TODO: Rename
func (c *Corpus) indexFile(p *Package, f *File, fset *token.FileSet) error {
	switch {
	case p.FindPackageFiles():
		name, ok := c.parseFileName(f.Path, fset)
		if !ok {
			return nil
		}
		switch p.Name {
		case "":
			p.Name = name
		case name:
			// Ok
		default:
			var firstFile *File
			for _, f := range p.GoFiles {
				firstFile = f
				break
			}
			return &MultiplePackageError{
				Dir:      p.Dir,
				Packages: []string{p.Name, name},
				Files:    []string{firstFile.Name, f.Name},
			}
		}
	case p.FindPackageName():
		if p.Name == "" {
			if name, ok := c.parseFileName(f.Path, fset); ok {
				p.Name = name
			}
		}
	}
	return nil
}

func (c *Corpus) parseFileName(path string, fset *token.FileSet) (string, bool) {
	src, err := c.readFile(path)
	if err != nil {
		return "", false
	}
	var name string
	af, _ := parser.ParseFile(fset, path, src, parser.PackageClauseOnly)
	if af != nil && af.Name != nil {
		name = af.Name.Name
	}
	return name, name != ""
}

func (c *Corpus) readFile(path string) ([]byte, error) {
	c.fsOpenGate <- true
	defer func() { <-c.fsOpenGate }()
	return ioutil.ReadFile(path)
}

func (c *Corpus) readdirnames(name string) ([]string, os.FileInfo, error) {
	c.fsOpenGate <- true
	defer func() { <-c.fsOpenGate }()
	f, err := os.Open(name)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, nil, err
	}
	names, err := f.Readdirnames(-1)
	if err != nil {
		return nil, nil, err
	}
	sort.Strings(names)
	return names, fi, nil
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
