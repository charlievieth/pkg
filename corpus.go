package pkg

import (
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

type Package struct {
	Root   string // root of Go tree where this package lives
	Path   string // directory path
	Name   string // package name
	Goroot bool   // package found in Go root

	GoFiles        []string // .go source files (excluding TestGoFiles, XTestGoFiles)
	IgnoredGoFiles []string // .go source files ignored for this build
	TestGoFiles    []string // _test.go files in package
}

func (c *Corpus) updatePackage(p *Package) {

}

func (c *Corpus) newPackage(dir string, fset *token.FileSet) *Package {
	names, err := readdirnames(dir)
	if err != nil {
		return nil // Change
	}
	// Exit early if the package is an executable.
	if sort.SearchStrings(names, "main.go") != len(names) {
		name, ok := parsePkgName(filepath.Join(dir, "main.go"), fset)
		if ok && name == "main" {
			return nil
		}
	}
	// Remove non-Go files
	names = FilterList(names, isGoFile)
	if len(names) == 0 {
		return nil
	}
	var (
		testGoFiles    []string
		goFiles        []string
		ignoredGoFiles []string
		pkgName        string
		isPkg          bool
	)
	for _, name := range names {
		switch {
		case !c.matchFile(dir, name):
			ignoredGoFiles = append(ignoredGoFiles, name)
		case isGoTestFile(name):
			testGoFiles = append(testGoFiles, name)
		default:
			goFiles = append(goFiles, name)
			if pkgName == "" {
				n, ok := parsePkgName(filepath.Join(dir, name), fset)
				if ok {
					isPkg = true
					pkgName = n
				}
			}
		}
	}
	if !isPkg {
		return nil
	}
	p := Package{
		Path:           dir,
		Name:           pkgName,
		TestGoFiles:    testGoFiles,
		GoFiles:        goFiles,
		IgnoredGoFiles: ignoredGoFiles,
	}
	for _, root := range c.ctxt.SrcDirs() {
		if sameRoot(root, dir) {
			p.Root = root
			p.Goroot = (root == c.ctxt.GOROOT)
			break
		}
	}
	return &p
}
