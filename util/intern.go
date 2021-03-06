package util

import "sync"

// A StringInterner is a string intern pool.
type StringInterner struct {
	sync.RWMutex
	strings map[string]string
}

func (x *StringInterner) get(s string) (string, bool) {
	if x.strings == nil {
		return "", false
	}
	x.RLock()
	s, ok := x.strings[s]
	x.RUnlock()
	return s, ok
}

// WARN: NEW!!!
func (x *StringInterner) lazyInit() {
	if x.strings == nil {
		x.Lock()
		if x.strings == nil {
			x.strings = make(map[string]string)
		}
		x.Unlock()
	}
}

// WARN: NEW!!!
func (x *StringInterner) intern(s string) string {
	x.lazyInit()
	x.RLock()
	si, ok := x.strings[s]
	x.RUnlock()
	if !ok {
		x.Lock()
		x.strings[si] = si
		x.Unlock()
	}
	return si
}

func (x *StringInterner) add(s string) string {
	x.Lock()
	if x.strings == nil {
		x.strings = make(map[string]string)
	}
	// Check if the string was added
	// before the lock was acquired.
	if si, ok := x.strings[s]; ok {
		s = si
	} else {
		x.strings[s] = s
	}
	x.Unlock()
	return s
}

// Intern, returns the interned string for s.
func (x *StringInterner) Intern(s string) string {
	if s, ok := x.get(s); ok {
		return s
	}
	return x.add(s)
}
