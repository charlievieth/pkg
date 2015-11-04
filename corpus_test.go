package pkg

import (
	"go/token"
	"os"
	"path/filepath"
	"testing"
)

func BenchmarkCorpus_IndexFiles(b *testing.B) {
	for i := 0; i < b.N; i++ {
		c := NewCorpus(FindPackageFiles, true)
		if err := c.Init(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCorpus_FindFiles(b *testing.B) {
	for i := 0; i < b.N; i++ {
		c := NewCorpus(FindPackageFiles, false)
		if err := c.Init(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCorpus_FindName(b *testing.B) {
	for i := 0; i < b.N; i++ {
		c := NewCorpus(FindPackageName, false)
		if err := c.Init(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCorpusUpdate_IndexFiles(b *testing.B) {
	c := NewCorpus(FindPackageFiles, true)
	c.Init()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Update()
	}
}

func BenchmarkCorpusUpdate_FindFiles(b *testing.B) {
	c := NewCorpus(FindPackageFiles, false)
	c.Init()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Update()
	}
}

func BenchmarkCorpusUpdate_FindName(b *testing.B) {
	c := NewCorpus(FindPackageName, false)
	c.Init()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Update()
	}
}

func BenchmarkCorpusUpdate_Package(b *testing.B) {
	c := NewCorpus(FindPackageFiles, true)
	c.Init()
	pkgName := "net/http"
	p, err := c.LookupPackage(filepath.Join(c.ctxt.GOROOT(), "src", pkgName))
	if err != nil {
		b.Fatal(err)
	}
	if p == nil {
		b.Fatalf("missing package:", "net/http")
	}
	fset := token.NewFileSet()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fi, err := os.Stat(p.Dir)
		if err != nil {
			b.Fatal(err)
		}
		c.updatePackage(p, fi, fset, nil)
	}
}
