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
	fset := token.NewFileSet()
	dirs := make(map[string]*Directory)
	c := &Corpus{
		ctxt:    ctxt,
		dirs:    dirs,
		srcDirs: srcDirs,
		mu:      sync.RWMutex{},
	}
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
	if c.dirs == nil || len(c.dirs) == 0 {
		return nil // []string{} ???
	}
	if fi, err := os.Stat(path); err == nil {
		if fi.IsDir() {
			path = filepath.Dir(path)
		} else {
			path = filepath.Clean(path)
		}
	}
	list := make([]string, 0, 1024)
	for _, d := range c.dirs {
		d.listPkgs(path, &list)
	}
	sort.Strings(list)
	c.mu.RUnlock()
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
	c.mu.Unlock()
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
