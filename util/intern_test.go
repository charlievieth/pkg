package util

import (
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
