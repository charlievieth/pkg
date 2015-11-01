package pkg

import (
	"go/token"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type Corpus struct {
	ctxt       *Context
	lastUpdate time.Time

	dirs map[string]*Directory

	MaxDepth int
	mu       sync.RWMutex

	PackageMode   ImportMode
	IndexFileInfo bool
	// WARN: New
	IndexEnabled bool
	IndexGoCode  bool
}

// TODO: Do we care about missing GOROOT and GOPATH env vars?
func NewCorpus(mode ImportMode, indexFileInfo bool) *Corpus {
	c := &Corpus{
		ctxt:          NewContext(nil, 0),
		dirs:          make(map[string]*Directory),
		PackageMode:   mode,
		IndexFileInfo: indexFileInfo,
	}
	return c
}

func (c *Corpus) Init() error {
	if err := c.initDirTree(); err != nil {
		return err
	}
	return nil
}

func (c *Corpus) initDirTree() error {
	dirs := c.ctxt.SrcDirs()
	for _, root := range dirs {
		dir, err := c.newDirectory(root, c.MaxDepth)
		if err != nil {
			return err
		}
		if dir != nil {
			if c.dirs == nil {
				c.dirs = make(map[string]*Directory)
			}
			c.dirs[root] = dir
		}
	}
	return nil
}

// WARN
func (c *Corpus) Dirs() map[string]*Directory {
	return c.dirs
}

// WARN: Remove LOCKS !!!

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
	for _, path := range c.ctxt.SrcDirs() {
		seen[path] = true
		fset := token.NewFileSet()
		if _, ok := c.dirs[path]; ok {
			c.dirs[path] = t.updateDirTree(c.dirs[path], fset)
		} else {
			c.dirs[path] = t.newDirTree(fset, path, filepath.Base(path), 0, false)
		}
	}

	// WARN: Do we want to remove directories?

	// Cleanup root directories
	for path := range seen {
		if !seen[path] {
			delete(c.dirs, path)
		}
	}
}
