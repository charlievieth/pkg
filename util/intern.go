package util

import (
	"sync"
)

// A StringInterner is a string intern pool.
type StringInterner struct {
	sync.RWMutex
	strings map[string]string
}

func (x *StringInterner) Intern(s string) string {
	x.Lock()
	if x.strings == nil {
		x.strings = make(map[string]string)
	}
	si, ok := x.strings[s]
	if !ok {
		x.strings[s] = s
		si = s
	}
	x.Unlock()
	return si
}
