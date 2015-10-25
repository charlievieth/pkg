package pkg

import (
	"fmt"
	"go/build"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
)

type Corpus struct {
	ctxt     *build.Context
	dirs     map[string]*Directory
	srcDirs  []string
	MaxDepth int
	mu       sync.RWMutex
}

// TODO: Do we care about missing GOROOT and GOPATH env vars?
func NewCorpus() *Corpus {
	ctxt, srcDirs := newContext()
	dirs := make(map[string]*Directory)
	c := &Corpus{
		ctxt:    ctxt,
		dirs:    dirs,
		srcDirs: srcDirs,
		mu:      sync.RWMutex{},
	}
	fset := token.NewFileSet()
	t := newTreeBuilder(c)
	for _, path := range srcDirs {
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
	c.updateContext()

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

func (c *Corpus) updateContext() {
	// Assumes Write-Lock
	if c.ctxt == nil {
		c.ctxt, c.srcDirs = newContext()
		return
	}
	path := os.Getenv("GOPATH")
	root := runtime.GOROOT()
	if path != c.ctxt.GOPATH || root != c.ctxt.GOROOT {
		c.ctxt.GOPATH = path
		c.ctxt.GOROOT = root
		c.srcDirs = c.ctxt.SrcDirs()
		for _, d := range c.srcDirs {
			if _, ok := c.dirs[d]; !ok {
				delete(c.dirs, d)
			}
		}
	}
}

func (c *Corpus) matchFile(dir, name string) bool {
	ok, err := c.ctxt.MatchFile(dir, name)
	return ok && err == nil
}

func newContext() (*build.Context, []string) {
	c := build.Default
	c.GOPATH = os.Getenv("GOPATH")
	c.GOROOT = runtime.GOROOT()
	return &c, c.SrcDirs()
}

type ImportMode int

const (
	// If FindPackage is set, NewPackage stops after reading ony package
	// statement.
	FindPackage ImportMode = 1 << iota

	// If CheckPackage is set, NewPackage checks all package statements.
	CheckPackage
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
}

// IsCommand reports whether the package is considered a command to be installed
// (not just a library). Packages named "main" are treated as commands.
func (p *Package) IsCommand() bool {
	return p.Name == "main"
}

func (c *Corpus) updatePackage(p *Package) {

}

func (c *Corpus) NewPackage(dir string) *Package {
	p, _ := c.newPackage(dir, token.NewFileSet(), CheckPackage)
	return p
}

func (c *Corpus) newPackage(dir string, fset *token.FileSet, mode ImportMode) (*Package, error) {
	names, err := readdirnames(dir)
	if err != nil {
		return nil, err
	}
	var (
		testGoFiles    []string
		goFiles        []string
		ignoredGoFiles []string
		pkgName        string
	)
	// Remove non-Go files
	names = FilterList(names, isGoFile)
	for _, name := range names {
		switch {
		case !c.matchFile(dir, name):
			ignoredGoFiles = append(ignoredGoFiles, name)
		case isGoTestFile(name):
			testGoFiles = append(testGoFiles, name)
		default:
			goFiles = append(goFiles, name)
			if mode&CheckPackage != 0 {
				if n, ok := parseFileName(filepath.Join(dir, name), fset); ok {
					switch pkgName {
					case n:
						// Ok
					case "":
						pkgName = n
					default:
						return nil, &MultiplePackageError{
							Dir:      dir,
							Packages: []string{pkgName, n},
							Files:    []string{goFiles[0], name},
						}
					}
				}
			} else {
				if pkgName == "" {
					n, ok := parsePkgName(filepath.Join(dir, name), fset)
					if ok {
						pkgName = n
					}
				}
			}
		}
	}
	p := Package{
		Dir:            dir,
		Name:           pkgName,
		TestGoFiles:    testGoFiles,
		GoFiles:        goFiles,
		IgnoredGoFiles: ignoredGoFiles,
	}
	if pkgName == "" {
		return &p, &NoGoError{Dir: dir}
	}
	// SrcDirs returns $GOPATH + "/src"
	for _, srcDir := range c.ctxt.SrcDirs() {
		if sameRoot(dir, srcDir) {
			p.ImportPath = trimPathPrefix(dir, srcDir)
			p.Root = filepath.Dir(srcDir)
			p.Goroot = sameRoot(dir, c.ctxt.GOROOT)
			break
		}
	}
	return &p, nil
}

func (c *Corpus) importPackage(dir string, fset *token.FileSet, mode ImportMode) (*Package, error) {
	names, err := readdirnames(dir)
	if err != nil {
		return nil, err
	}
	p := Package{
		Dir: dir,
	}
	// SrcDirs returns $GOPATH + "/src"
	for _, srcDir := range c.ctxt.SrcDirs() {
		if sameRoot(dir, srcDir) {
			p.ImportPath = trimPathPrefix(dir, srcDir)
			p.Root = filepath.Dir(srcDir)
			p.Goroot = sameRoot(dir, c.ctxt.GOROOT)
			break
		}
	}
	for _, name := range FilterList(names, isGoFile) {
		switch {
		case !c.matchFile(dir, name):
			p.IgnoredGoFiles = append(p.IgnoredGoFiles, name)
		case isGoTestFile(name):
			p.TestGoFiles = append(p.TestGoFiles, name)
		default:
			// TODO (CEV): Check file before adding based on mode?
			p.GoFiles = append(p.GoFiles, name)
			if mode&CheckPackage != 0 {
				if n, ok := parseFileName(filepath.Join(dir, name), fset); ok {
					switch p.Name {
					case n:
						// Ok
					case "":
						p.Name = n
					default:
						return nil, &MultiplePackageError{
							Dir:      dir,
							Packages: []string{p.Name, n},
							Files:    []string{p.GoFiles[0], name},
						}
					}
				}
			} else {
				if p.Name == "" {
					n, ok := parsePkgName(filepath.Join(dir, name), fset)
					if ok {
						p.Name = n
					}
				}
			}
		}
	}
	var pkgErr error
	if p.Name == "" {
		pkgErr = &NoGoError{Dir: dir}
	}
	return &p, pkgErr
}

func (p *Package) addFile(c *Corpus, fset *token.FileSet, dir, name string, mode ImportMode) error {
	if !c.matchFile(dir, name) {
		p.IgnoredGoFiles = append(p.IgnoredGoFiles, name)
		return nil
	}
	if isGoTestFile(name) {
		p.TestGoFiles = append(p.TestGoFiles, name)
		return nil
	}
	p.GoFiles = append(p.GoFiles, name)
	if mode&CheckPackage != 0 {
		if n, ok := parseFileName(filepath.Join(dir, name), fset); ok {
			switch p.Name {
			case n:
				// Ok
			case "":
				p.Name = n
			default:
				return &MultiplePackageError{
					Dir:      dir,
					Packages: []string{p.Name, n},
					Files:    []string{p.GoFiles[0], name},
				}
			}
		}
		return nil
	}
	if p.Name == "" {
		n, ok := parsePkgName(filepath.Join(dir, name), fset)
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
