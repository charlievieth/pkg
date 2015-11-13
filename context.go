package pkg

import (
	"fmt"
	"go/build"
	"os"
	pathpkg "path"
	"runtime"
	"sync"
	"time"

	"git.vieth.io/pkg/fs"
)

type Context struct {
	ctxt           *build.Context
	srcDirs        []string
	lastUpdate     time.Time
	updateInterval time.Duration // ignored if less than or equal to zero
	mu             sync.RWMutex
}

func NewContext(ctxt *build.Context, updateInterval time.Duration) *Context {
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
	if s := clean(s); fs.IsDir(s) {
		c.doUpdate(s, c.GOPATH())
	}
}

func (c *Context) SetGoPath(s string) {
	if s := clean(s); fs.IsDir(s) {
		c.doUpdate(c.GOROOT(), s)
	}
}

// PkgTargetRoot, returns the package directory and package .a file for the
// Go package named by the import path and the current context.
//
// See: go/build/build.go Import() for more information.
func (c *Context) PkgTargetRoot(path string) (pkgRoot string, pkgA string, err error) {
	ctxt := c.Context()
	suffix := ctxt.InstallSuffix
	if suffix != "" {
		suffix = "_" + suffix
	}
	switch ctxt.Compiler {
	case "gccgo":
		pkgRoot = "pkg/gccgo_" + ctxt.GOOS + "_" + ctxt.GOARCH + suffix
		dir, elem := pathpkg.Split(path)
		pkgA = pkgRoot + "/" + dir + "lib" + elem + ".a"
	case "gc":
		pkgRoot = "pkg/" + ctxt.GOOS + "_" + ctxt.GOARCH + suffix
		pkgA = pkgRoot + "/" + path + ".a"
	default:
		err = fmt.Errorf("pkg: unknown compiler %q", ctxt.Compiler)
	}
	return pkgRoot, pkgA, err
}

func (c *Context) MatchFile(dir, name string) bool {
	ok, err := c.Context().MatchFile(dir, name)
	return ok && err == nil
}

func (c *Context) Update() {
	if c.ctxt == nil || c.srcDirs == nil || c.outdated() {
		c.doUpdate(runtime.GOROOT(), os.Getenv("GOPATH"))
	}
}

func (c *Context) outdated() bool {
	if c.updateInterval <= 0 {
		return false
	}
	c.mu.RLock()
	update := time.Since(c.lastUpdate) >= c.updateInterval
	c.mu.RUnlock()
	return update
}

func (c *Context) doUpdate(root, path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
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
}

func (c *Context) initDefault() {
	ctxt := build.Default
	ctxt.GOPATH = os.Getenv("GOPATH")
	ctxt.GOROOT = runtime.GOROOT()
	c.ctxt = &ctxt
	c.srcDirs = ctxt.SrcDirs()
}
