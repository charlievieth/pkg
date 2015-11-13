package pkg

import (
	"log"
	"os"
	"sync"
	"time"
)

type Corpus struct {
	ctxt          *Context
	MaxDepth      int
	LogEvents     bool
	IndexGoCode   bool
	IndexThrottle float64
	IndexInterval time.Duration
	log           *log.Logger
	idents        *Index
	packages      *PackageIndex
	dirs          map[string]*Directory
	eventCh       chan Eventer
	mu            sync.RWMutex
	lastUpdate    time.Time
}

// TODO: Do we care about missing GOROOT and GOPATH env vars?
func NewCorpus() *Corpus {
	logger := log.New(os.Stdout, "", log.LstdFlags)
	c := &Corpus{
		ctxt:        NewContext(nil, 0),
		dirs:        make(map[string]*Directory),
		MaxDepth:    defaultMaxDepth,
		IndexGoCode: true,
		LogEvents:   false,
		log:         logger,
		eventCh:     make(chan Eventer, 200),
	}
	return c
}

func (c *Corpus) EventStream() {
	for e := range c.eventCh {
		c.log.Println(e.String())
		if err := e.Callback(c); err != nil {
			_ = err
		}
	}
}

func (c *Corpus) lazyInitEventChan() {
	if c.eventCh == nil {
		c.mu.Lock()
		if c.eventCh == nil {
			c.eventCh = make(chan Eventer, 200)
		}
		c.mu.Unlock()
	}
}

func (c *Corpus) notify(e Eventer) {
	if !c.LogEvents {
		return
	}
	c.lazyInitEventChan()
	select {
	case c.eventCh <- e:
	case <-time.After(time.Millisecond * 100):
		if c.LogEvents {
			c.log.Println("Corpus: sending event timed out")
		}
	}
}

func (c *Corpus) Init() error {
	if c.packages == nil {
		c.packages = newPackageIndex(c)
	}
	if c.IndexGoCode {
		c.idents = newIndex(c)
	}
	if err := c.initDirTree(); err != nil {
		return err
	}
	return nil
}

// WARN
func (c *Corpus) Update() {
	for root, dir := range c.dirs {
		t := newTreeBuilder(c, c.MaxDepth)
		dir = t.updateDirTree(dir)
		if dir == nil {
			panic("NIL DIR " + root)
		}
	}
}

// initDirTree, initializes the Directory tree's at build.Context.SrcDirs().
// An error is returned if root is not a directory or there was an error
// statting it.
func (c *Corpus) initDirTree() error {
	srcDirs := c.ctxt.SrcDirs()
	for _, root := range srcDirs {
		if dir := c.newDirectory(root, c.MaxDepth); dir != nil {
			c.dirs[root] = dir
		}
	}
	return nil
}

func (c *Corpus) newDirectory(root string, maxDepth int) *Directory {
	t := newTreeBuilder(c, maxDepth)
	fi, err := os.Stat(root)
	if err != nil || !fi.IsDir() {
		return nil
	}
	return t.newDirTree(root, fi, 0, false)
}

// WARN
func (c *Corpus) Packages() map[string]map[string]*Package {
	return c.packages.packages
}

// WARN
func (c *Corpus) Dirs() map[string]*Directory {
	return c.dirs
}

func (c *Corpus) DirList() map[string]*DirList {
	m := make(map[string]*DirList)
	for root, dir := range c.dirs {
		m[root] = dir.listing(true, nil)
	}
	return m
}
