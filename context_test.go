package pkg2

import (
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

func TestContextUpdate(t *testing.T) {
	if testing.Short() {
		t.Skip("Context Update: skipped test")
	}
	t.Parallel()
	c := NewContext(nil, time.Minute)
	for i := 0; i < 40; i++ {
		go func() {
			for {
				c.doUpdate(randPaths())
			}
		}()
	}
	wg := new(sync.WaitGroup)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for i := 0; i < 100000; i++ {
				root := c.GOROOT()
				if !validpaths[root] {
					t.Errorf("root: (%s) proc: (%d) loop: (%d)", root, n, i)
				}
			}
		}(i)
	}
	wg.Wait()
}

func BenchmarkGOROOT(b *testing.B) {
	c := NewContext(nil, time.Minute)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.GOROOT()
	}
}
