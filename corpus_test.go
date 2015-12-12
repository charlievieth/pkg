package pkg

import (
	"testing"
	"time"
)

func BenchmarkCorpusInit(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		c := NewCorpus()
		c.IndexGoCode = false
		c.LogEvents = false
		if err := c.Init(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCorpusUpdate(b *testing.B) {
	c := NewCorpus()
	c.IndexGoCode = false
	c.LogEvents = false
	c.IndexInterval = time.Hour
	if err := c.Init(); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		c.updateIndex()
	}
}
