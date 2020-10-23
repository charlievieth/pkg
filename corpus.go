package pkg

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"
	"time"
)

// TODO:
//  - Add fields for ignoring directories.
//  - Improve Corpus creation (Context) and defaults.
//  - Remove unused fields

type Corpus struct {
	ctxt               *Context
	MaxDepth           int
	LogEvents          bool
	IndexGoCode        bool
	IndexThrottle      float64
	IndexInterval      time.Duration
	log                *log.Logger
	idents             *Index
	packages           *PackageIndex
	dirs               map[string]*Directory
	lastUpdate         time.Time
	eventCh            chan Eventer
	refreshIndexSignal chan bool
	stop               chan bool
	mu                 sync.RWMutex
	wg                 sync.WaitGroup
}

func (c Corpus) MarshalJSON() ([]byte, error) {
	type CorpusExt struct {
		Context       *Context
		MaxDepth      int
		LogEvents     bool
		IndexGoCode   bool
		IndexThrottle float64
		IndexInterval time.Duration
		Idents        *Index
		Packages      *PackageIndex
		Dirs          map[string]*Directory
		LastUpdate    time.Time
	}
	ext := CorpusExt{
		Context:       c.ctxt,
		MaxDepth:      c.MaxDepth,
		LogEvents:     c.LogEvents,
		IndexGoCode:   c.IndexGoCode,
		IndexThrottle: c.IndexThrottle,
		IndexInterval: c.IndexInterval,
		Idents:        c.idents,
		Packages:      c.packages,
		Dirs:          c.dirs,
		LastUpdate:    c.lastUpdate,
	}
	return json.Marshal(&ext)
}

// TODO: Do we care about missing GOROOT and GOPATH env vars?
func NewCorpus() *Corpus {
	logger := log.New(os.Stdout, "", log.LstdFlags)
	c := &Corpus{
		ctxt:               NewContext(nil, 0),
		dirs:               make(map[string]*Directory),
		MaxDepth:           defaultMaxDepth,
		IndexGoCode:        true,
		LogEvents:          false,
		log:                logger,
		eventCh:            make(chan Eventer, 100),
		refreshIndexSignal: make(chan bool, 1), // buffer
		IndexInterval:      time.Second * 3,
	}
	return c
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
	if !c.LogEvents || e == nil {
		return
	}
	c.lazyInitEventChan()
	select {
	case c.eventCh <- e:
		// Ok
	case <-c.stop:
		// Don't send
	case <-time.After(time.Second):
		c.log.Println("\033[31mCorpus: sending event timed out\033[0m")
	}
}

func (c *Corpus) eventStream() {
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		for {
			select {
			case e := <-c.eventCh:
				if !c.LogEvents {
					break
				}
				c.log.Println(e.String())
				if err := e.Callback(c); err != nil {
					// TODO: Add more info to event
					c.log.Printf("Error: %s")
				}
			case <-c.stop:
				return
			}
		}
	}()
}

func (c *Corpus) refreshIndex() {
	select {
	case c.refreshIndexSignal <- true:
	default:
	}
}

func (c *Corpus) refreshIndexLoop() {
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		lastUpdate := time.Now()
		for {
			select {
			case <-c.refreshIndexSignal:
				if time.Since(lastUpdate) >= time.Second {
					start := time.Now()
					c.updateIndex()
					e := Event{
						typ: UpdateEvent,
						msg: fmt.Sprintf("Index: \033[33mupdated\033[0m in %s", time.Since(start)),
					}
					c.notify(&e)
					lastUpdate = time.Now()
				}
			case <-time.After(c.IndexInterval):
				if time.Since(lastUpdate) >= time.Second {
					start := time.Now()
					c.updateIndex()
					e := Event{
						typ: UpdateEvent,
						msg: fmt.Sprintf("Index: \033[33mupdated\033[0m in %s", time.Since(start)),
					}
					c.notify(&e)
					lastUpdate = time.Now()
				}
			case <-c.stop:
				return
			}
		}
	}()
}

func (c *Corpus) updateIndex() {
	srcDirs := c.ctxt.SrcDirs()
	seen := make(map[string]bool)
	for _, root := range srcDirs {
		seen[root] = true
		var d *Directory
		if dir := c.dirs[root]; dir != nil {
			d = newTreeBuilder(c, c.MaxDepth).updateDirTree(dir)
		} else {
			d = c.newDirectory(root, c.MaxDepth)
		}
		if d != nil {
			c.dirs[root] = d
		} else {
			delete(c.dirs, root)
		}
	}
	// Remove missing directories
	for root := range c.dirs {
		if !seen[root] {
			delete(c.dirs, root)
		}
	}
}

func (c *Corpus) Init() error {
	logEvents := c.LogEvents
	c.LogEvents = false
	c.eventStream()
	if c.packages == nil {
		c.packages = newPackageIndex(c)
	}
	if c.IndexGoCode {
		c.idents = newIndex(c)
	}
	if err := c.initDirTree(); err != nil {
		return err
	}
	c.LogEvents = logEvents
	c.refreshIndexLoop()
	return nil
}

func (c *Corpus) Stop() {
	select {
	case <-c.stop:
		c.log.Println("Corpus: index not running!")
	default:
		c.log.Println("Corpus: stopping index.")
	}
	t := time.Now()
	close(c.stop)
	c.wg.Wait()
	c.log.Printf("Corpus: shutdown complete, elapsed time: %s", time.Since(t))
}

// WARN
func (c *Corpus) Update() {
	if c.packages == nil {
		c.packages = newPackageIndex(c)
	}
	if c.IndexGoCode {
		c.idents = newIndex(c)
	}
	c.updateIndex()

	// for root, dir := range c.dirs {
	// 	t := newTreeBuilder(c, c.MaxDepth)
	// 	dir = t.updateDirTree(dir)
	// 	if dir == nil {
	// 		panic("NIL DIR " + root)
	// 	}
	// }
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

// WARN
func (c *Corpus) Idents() []Ident {
	if c.idents == nil {
		return nil
	}
	return c.idents.Idents()
}

func (c *Corpus) DirList() map[string]*DirList {
	m := make(map[string]*DirList)
	for root, dir := range c.dirs {
		m[root] = dir.listing(true, nil)
	}
	return m
}
