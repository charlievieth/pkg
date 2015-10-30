package pkg

import (
	"go/build"
	"os"
	"runtime"
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
	c.Update()
	return c.ctxt
}

func (c *Context) SrcDirs() []string {
	c.Update()
	return c.srcDirs
}

func (c *Context) GOROOT() string {
	return c.Context().GOROOT
}

func (c *Context) GOPATH() string {
	return c.Context().GOPATH
}

func (c *Context) SetGoRoot(s string) {
	c.doUpdate(s, c.GOPATH())
}

func (c *Context) SetGoPath(s string) {
	c.doUpdate(c.GOROOT(), s)
}

func (c *Context) MatchFile(dir, name string) bool {
	ok, err := c.Context().MatchFile(dir, name)
	return ok && err == nil
}

func (c *Context) Update() {
	if c.ctxt == nil || c.outdated() {
		c.doUpdate(runtime.GOROOT(), os.Getenv("GOPATH"))
	}
}

func (c *Context) outdated() bool {
	c.mu.RLock()
	update := time.Since(c.lastUpdate) >= c.updateInterval
	c.mu.RUnlock()
	return update
}

func (c *Context) doUpdate(root, path string) {
	c.mu.Lock()
	c.lastUpdate = time.Now()
	switch {
	case c.ctxt == nil:
		c.initDefault()
	case path != c.ctxt.GOPATH || root != c.ctxt.GOROOT:
		// Copy and replace on change
		ctxt := *c.ctxt
		ctxt.GOPATH = path
		ctxt.GOROOT = root
		srcDirs := ctxt.SrcDirs()
		c.ctxt = &ctxt
		c.srcDirs = srcDirs
	case len(c.srcDirs) == 0:
		if c.ctxt.GOROOT != "" || c.ctxt.GOPATH != "" {
			c.srcDirs = c.ctxt.SrcDirs()
		}
	}
	c.mu.Unlock()
}

func (c *Context) initDefault() {
	ctxt := build.Default
	ctxt.GOPATH = os.Getenv("GOPATH")
	ctxt.GOROOT = runtime.GOROOT()
	c.ctxt = &ctxt
	c.srcDirs = ctxt.SrcDirs()
}
