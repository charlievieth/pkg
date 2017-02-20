package util

import (
	crand "crypto/rand"
	"encoding/base64"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"unsafe"
)

func TestStringInternerInit(t *testing.T) {
	defer func() {
		if e := recover(); e != nil {
			t.Fatal(e)
		}
	}()
	var i StringInterner
	i.Intern("a")
}

// Test that the string is actually interned, by comparing the
// underlying data pointers of the returned strings.
func TestStringInterner(t *testing.T) {
	var i StringInterner
	s1 := "a"
	s2 := i.Intern("a")
	p1 := *(*uintptr)(unsafe.Pointer(&s1))
	p2 := *(*uintptr)(unsafe.Pointer(&s2))
	if p1 != p2 {
		t.Fatalf("TestStringInterner pointer: %p %p", s1, s2)
	}
}

var RandomStrings []string

func init() {
	r := rand.New(rand.NewSource(1))
	for i := 0; i < 512; i++ {
		b := make([]byte, r.Intn(256))
		if _, err := crand.Read(b); err != nil {
			panic(err)
		}
		s := base64.StdEncoding.EncodeToString(b)
		RandomStrings = append(RandomStrings, s)
	}
}

func BenchmarkRead(b *testing.B) {
	var x StringInterner
	for _, s := range RandomStrings {
		x.Intern(s)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		x.Intern(RandomStrings[i%len(RandomStrings)])
	}
}

func BenchmarkRead_Parallel(b *testing.B) {
	var x StringInterner
	for _, s := range RandomStrings {
		x.Intern(s)
	}
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		var n int
		for pb.Next() {
			x.Intern(RandomStrings[n%len(RandomStrings)])
			n++
		}
	})
}

func BenchmarkWrite(b *testing.B) {
	var x StringInterner
	var n int
	for i := 0; i < b.N; i++ {
		x.Intern(RandomStrings[i%len(RandomStrings)])
		n++
		if n == len(RandomStrings) {
			n = 0
			x.strings = nil
		}
	}
}

func BenchmarkWrite_Parallel(b *testing.B) {
	var x StringInterner
	var n uint32
	var mu sync.RWMutex
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			i := int(atomic.AddUint32(&n, 1))
			mu.RLock()
			x.Intern(RandomStrings[i%len(RandomStrings)])
			mu.RUnlock()
			if i == len(RandomStrings) {
				mu.Lock()
				atomic.StoreUint32(&n, 0)
				x.strings = nil
				mu.Unlock()
			}
		}
	})
}
