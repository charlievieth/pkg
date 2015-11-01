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

	dirs     map[string]*Directory
	packages map[string]map[string]*Package // "GOPATH" => "net/http" => Pkg "http"

	MaxDepth int
	mu       sync.RWMutex

	PackageMode   ImportMode
	IndexFileInfo bool
	// WARN: New
	IndexEnabled bool
	IndexGoCode  bool

	index *Indexer
}

// TODO: Do we care about missing GOROOT and GOPATH env vars?
func NewCorpus(mode ImportMode, indexFileInfo bool) *Corpus {
	c := &Corpus{
		ctxt:          NewContext(nil, 0),
		dirs:          make(map[string]*Directory),
		MaxDepth:      512,
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

// initDirTree, initializes the Directory tree's at build.Context.SrcDirs().
// An error is returned if root is not a directory or there was an error
// statting it.
func (c *Corpus) initDirTree() error {
	srcDirs := c.ctxt.SrcDirs()
	for _, root := range srcDirs {
		if err := c.updateDirTree(root); err != nil {
			return err
		}
	}
	return nil
}

// updateDirTree, updates the Directory tree at root.  If no Directory tree is
// currently stored at root - one is created.  An error is returned if root is
// not a directory or there was an error statting it.
func (c *Corpus) updateDirTree(root string) error {
	if c.dirs == nil {
		c.dirs = make(map[string]*Directory)
	}
	var (
		dir *Directory
		err error
	)
	if dir = c.dirs[root]; dir != nil {
		dir, err = c.updateDirectory(dir, c.MaxDepth)
	} else {
		dir, err = c.newDirectory(root, c.MaxDepth)
	}
	if dir != nil {
		c.dirs[root] = dir
	} else {
		delete(c.dirs, root)
	}
	return err
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

// WARN
func (c *Corpus) Dirs() map[string]*Directory {
	return c.dirs
}

// WARN: Remove LOCKS !!!
//
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

// WARN: Dev only
func (c *Corpus) Index() *Indexer {
	return c.index
}

func (c *Corpus) InitIndex() {
	if c.index == nil {
		c.index = newIndexer(c)
	}
	for _, d := range c.dirs {
		c.index.indexDirectory(d)
	}
}
