package pkg

import (
	"bytes"
	"testing"
)

func BenchmarkBufferPool(b *testing.B) {
	for i := 0; i < 10; i++ {
		putBuffer(new(bytes.Buffer))
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b := getBuffer()
		putBuffer(b)
	}
}
