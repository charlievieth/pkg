package pkg

import (
	"testing"
)

func TestTrimPathPrefix(t *testing.T) {
	// Sanity check
	path := "/usr/local/go/src/bufio"
	root := "/usr/local/go/src"
	exp := "bufio"
	if s := trimPathPrefix(path, root); s != exp {
		t.Errorf("trimPathPrefix: exp (%s) got (%s)", exp, s)
	}
}
