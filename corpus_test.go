package pkg

import (
	"testing"
)

func BenchmarkCorpusUpdate_IndexFiles(b *testing.B) {
	c := NewCorpus(FindPackageFiles, true)
	c.Init()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Update()
	}
}

func BenchmarkCorpusUpdate_FindFiles(b *testing.B) {
	c := NewCorpus(FindPackageFiles, true)
	c.Init()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Update()
	}
}

func BenchmarkCorpusUpdate_FindName(b *testing.B) {
	c := NewCorpus(FindPackageName, true)
	c.Init()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Update()
	}
}

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
