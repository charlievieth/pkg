package pkg

import (
	"go/build"
	"math/rand"
	"runtime"
	"sync"
	"testing"
	"time"
)

var validpaths map[string]bool

func init() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	validpaths = make(map[string]bool)
	for _, s := range filepaths {
		validpaths[s] = true
	}
}

// Random paths
var filepaths = []string{
	runtime.GOROOT(), // Actual Goroot
	"/usr/local/go/src/marchive",
	"/usr/local/go/src/mbufio",
	"/usr/local/go/src/mbuiltin",
	"/usr/local/go/src/mbytes",
	"/usr/local/go/src/mcmd",
	"/usr/local/go/src/mcompress",
	"/usr/local/go/src/mcontainer",
	"/usr/local/go/src/mcrypto",
	"/usr/local/go/src/mdatabase",
	"/usr/local/go/src/mdebug",
	"/usr/local/go/src/mencoding",
	"/usr/local/go/src/merrors",
	"/usr/local/go/src/mexpvar",
	"/usr/local/go/src/mflag",
	"/usr/local/go/src/mfmt",
	"/usr/local/go/src/mgo",
	"/usr/local/go/src/mhash",
	"/usr/local/go/src/mhtml",
	"/usr/local/go/src/mimage",
	"/usr/local/go/src/mindex",
	"/usr/local/go/src/minternal",
	"/usr/local/go/src/mio",
	"/usr/local/go/src/mlog",
	"/usr/local/go/src/make.bat",
	"/usr/local/go/src/mmath",
	"/usr/local/go/src/mmime",
	"/usr/local/go/src/mnet",
	"/usr/local/go/src/mos",
	"/usr/local/go/src/mpath",
	"/usr/local/go/src/race.bat",
	"/usr/local/go/src/mreflect",
	"/usr/local/go/src/mregexp",
	"/usr/local/go/src/run.bat",
	"/usr/local/go/src/mruntime",
	"/usr/local/go/src/msort",
	"/usr/local/go/src/mstrconv",
	"/usr/local/go/src/mstrings",
	"/usr/local/go/src/msync",
	"/usr/local/go/src/msyscall",
	"/usr/local/go/src/mtesting",
	"/usr/local/go/src/mtext",
	"/usr/local/go/src/mtime",
	"/usr/local/go/src/municode",
	"/usr/local/go/src/munsafe",
}

func randPaths() (string, string) {
	return filepaths[rand.Intn(len(filepaths))], filepaths[rand.Intn(len(filepaths))]
}

// Update stress-test to ensure Context properly handles concurrent access
// and updates.
func TestContextUpdate(t *testing.T) {
	if testing.Short() {
		t.Skip("Context Update: skipped test")
	}

	defer func() {
		if e := recover(); e != nil {
			t.Fatalf("Context Update Panic: %v", e)
		}
	}()

	t.Parallel()
	c := NewContext(nil, time.Minute)

	// Start update goroutines.
	done := make(chan struct{})
	defer close(done)

	for i := 0; i < runtime.NumCPU(); i++ {
		go func() {
			for {
				select {
				case <-done:
					return
				default:
					c.doUpdate(randPaths())
				}
			}
		}()
	}

	// Call GOROOT method while simultaneously updating the Context.
	wg := new(sync.WaitGroup)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for i := 0; i < 10000; i++ {
				root := c.GOROOT()
				if !validpaths[root] {
					t.Errorf("root: (%s) proc: (%d) loop: (%d)", root, n, i)
				}
			}
		}(i)
	}
	wg.Wait()
}

func TestContextPkgTargetRoot(t *testing.T) {

	defaultContext := func() *build.Context {
		return &build.Context{
			GOROOT:        build.Default.GOROOT,
			GOPATH:        build.Default.GOPATH,
			GOOS:          "darwin",
			GOARCH:        "amd64",
			Compiler:      "gc",
			InstallSuffix: "",
		}
	}

	{ // No suffix
		var (
			pkgName = "bytes"
			expRoot = "pkg/darwin_amd64"
			expA    = expRoot + "/" + pkgName + ".a"
		)
		ctxt := defaultContext()
		c := NewContext(ctxt, -1)

		pkgRoot, pkgA, err := c.PkgTargetRoot(pkgName)
		if err != nil {
			t.Fatalf("PkgTargetRoot (%+v): %v", ctxt, err)
		}
		if expRoot != pkgRoot {
			t.Errorf("PkgTargetRoot: Root Exp (%v) Got (%v)", expRoot, pkgRoot)
		}
		if expA != pkgA {
			t.Errorf("PkgTargetRoot: A Exp (%v) Got (%v)", expA, pkgA)
		}
	}

	{ // 'race' suffix
		var (
			suffix  = "race"
			pkgName = "bytes"
			expRoot = "pkg/darwin_amd64_race"
			expA    = expRoot + "/" + pkgName + ".a"
		)
		ctxt := defaultContext()
		ctxt.InstallSuffix = suffix

		c := NewContext(ctxt, -1)
		pkgRoot, pkgA, err := c.PkgTargetRoot(pkgName)
		if err != nil {
			t.Fatalf("PkgTargetRoot (%+v): %v", ctxt, err)
		}
		if expRoot != pkgRoot {
			t.Errorf("PkgTargetRoot: Root Exp (%v) Got (%v)", expRoot, pkgRoot)
		}
		if expA != pkgA {
			t.Errorf("PkgTargetRoot: A Exp (%v) Got (%v)", expA, pkgA)
		}
	}

	{ // gccgo
		var (
			compiler = "gccgo"
			pkgName  = "bytes"
			expRoot  = "pkg/gccgo_darwin_amd64"
			expA     = expRoot + "/" + "lib" + pkgName + ".a"
		)
		ctxt := defaultContext()
		ctxt.Compiler = compiler

		c := NewContext(ctxt, -1)
		pkgRoot, pkgA, err := c.PkgTargetRoot(pkgName)
		if err != nil {
			t.Fatalf("PkgTargetRoot (%+v): %v", ctxt, err)
		}
		if expRoot != pkgRoot {
			t.Errorf("PkgTargetRoot: Root Exp (%v) Got (%v)", expRoot, pkgRoot)
		}
		if expA != pkgA {
			t.Errorf("PkgTargetRoot: A Exp (%v) Got (%v)", expA, pkgA)
		}
	}
}

func BenchmarkGOROOT(b *testing.B) {
	c := NewContext(nil, time.Minute)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.GOROOT()
	}
}
