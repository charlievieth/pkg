package pkg

import (
	"git.vieth.io/pkg/fs"
	"testing"
)

func newDirtreeCorpus() {

}

func BenchmarkNewDirTree(b *testing.B) {
	c := NewCorpus()
	root := c.ctxt.GOROOT()
	if root == "" {
		b.Skip("GOROOT must be set to run benchmark")
		return
	}
	c.IndexGoCode = false
	c.LogEvents = false
	c.packages = newPackageIndex(c)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		fi, err := fs.Stat(root)
		if err != nil {
			b.Fatal(err)
		}
		newTreeBuilder(c, c.MaxDepth).newDirTree(root, fi, 0, false)
	}
}

func BenchmarkUpdateDirTree(b *testing.B) {
	c := NewCorpus()
	root := c.ctxt.GOROOT()
	if root == "" {
		b.Skip("GOROOT must be set to run benchmark")
		return
	}
	c.IndexGoCode = false
	c.LogEvents = false
	c.packages = newPackageIndex(c)
	t := newTreeBuilder(c, c.MaxDepth)
	fi, err := fs.Stat(root)
	if err != nil {
		b.Fatal(err)
	}
	dir := t.newDirTree(root, fi, 0, false)
	if dir == nil {
		b.Fatalf("BenchmarkUpdateDirTree: nil dir for %s", root)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		t.updateDirTree(dir)
	}
}
