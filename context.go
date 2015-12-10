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

// A Context provides safe-concurrent access to a build.Context, that optionally
// checks for changes to Go specific environment variables (GOROOT, GOPATH).
type Context struct {
	ctxt           *build.Context
	srcDirs        []string
	lastUpdate     time.Time
	updateInterval time.Duration // ignored if less than or equal to zero
	mu             sync.RWMutex
}

// NewContext, returns a new Context for build.Context ctxt with an update
// interval of updateInterval.  If updateInterval is less than or equal to
// zero the returned Context will not check the environment for changes to
// GOROOT and GOPATH.
//
// If ctxt is nil, build.Default and the current GOROOT and GOPATH are used.
func NewContext(ctxt *build.Context, updateInterval time.Duration) *Context {
	c := &Context{
		ctxt:           ctxt,
		updateInterval: updateInterval,
	}
	c.Update()
	return c
}

// Context returns a pointer the the current build.Context.  The returned
// *build.Context should *not* be modified by the reciever.
func (c *Context) Context() *build.Context {
	c.Update()
	return c.ctxt
}

// SrcDirs returns a list of package source root directories.  It draws from
// the current Go root and Go path but omits directories that do not exist.
//
// Results are cached for efficieny and only updated when GOROOT or GOPATH
// change.
func (c *Context) SrcDirs() []string {
	c.Update()
	return c.srcDirs
}

// GOROOT returns the GOROOT of Context.
func (c *Context) GOROOT() string {
	return c.Context().GOROOT
}

// GOPATH returns the GOPATH of Context.
func (c *Context) GOPATH() string {
	return c.Context().GOPATH
}

// SetGoRoot sets the Context GOROOT.
func (c *Context) SetGoRoot(s string) {
	if s := clean(s); fs.IsDir(s) {
		c.doUpdate(s, c.GOPATH())
	}
}

// SetGoPath sets the Context GOPATH.
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

// MatchFile reports whether the file with the given name in the given directory
// matches the context and would be included in a Package created by ImportDir
// of that directory.
//
// MatchFile considers the name of the file and may use ctxt.OpenFile to
// read some or all of the file's content.
//
// See: go/build/build.go Context.MatchFile for more information.
func (c *Context) MatchFile(dir, name string) bool {
	ok, err := c.Context().MatchFile(dir, name)
	return ok && err == nil
}

// Update, updates or initializes a Context that is outdated or has a nil
// build.Context or SrcDirs.
func (c *Context) Update() {
	if c.ctxt == nil || c.srcDirs == nil || c.outdated() {
		c.doUpdate(runtime.GOROOT(), os.Getenv("GOPATH"))
	}
}

// outdated returns if the Context is outdated and should be updated.  If the
// updateInterval is less than or equal to zero, false is always returned.
func (c *Context) outdated() bool {
	if c.updateInterval <= 0 {
		return false
	}
	c.mu.RLock()
	update := time.Since(c.lastUpdate) >= c.updateInterval
	c.mu.RUnlock()
	return update
}

// doUpdate, updates the current GOROOT and GOPATH to root and path.
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

// initDefault, initializes the Context to build.Default.
func (c *Context) initDefault() {
	ctxt := build.Default
	ctxt.GOPATH = os.Getenv("GOPATH")
	ctxt.GOROOT = runtime.GOROOT()
	c.ctxt = &ctxt
	c.srcDirs = ctxt.SrcDirs()
}
