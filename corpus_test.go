package pkg

import (
	"testing"
)

func BenchmarkCorpus_IndexFiles(b *testing.B) {
	for i := 0; i < b.N; i++ {
		NewCorpus(FindPackageFiles, true)
	}
}

func BenchmarkCorpus_FindFiles(b *testing.B) {
	for i := 0; i < b.N; i++ {
		NewCorpus(FindPackageFiles, false)
	}
}

func BenchmarkCorpus_FindName(b *testing.B) {
	for i := 0; i < b.N; i++ {
		NewCorpus(FindPackageName, false)
	}
}

func BenchmarkCorpus_Fast(b *testing.B) {
	for i := 0; i < b.N; i++ {
		NewCorpus(FindPackageOnly, false)
	}
}

func BenchmarkCorpusUpdate_IndexFiles(b *testing.B) {
	c := NewCorpus(FindPackageFiles, true)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Update()
	}
}

func BenchmarkCorpusUpdate_FindFiles(b *testing.B) {
	c := NewCorpus(FindPackageFiles, false)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Update()
	}
}

func BenchmarkCorpusUpdate_FindName(b *testing.B) {
	c := NewCorpus(FindPackageName, false)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Update()
	}
}

func BenchmarkCorpusUpdate_Fast(b *testing.B) {
	c := NewCorpus(FindPackageOnly, false)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Update()
	}
}
