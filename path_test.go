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

func TestHasPrefix(t *testing.T) {
	var tests = []struct {
		Path   string
		Prefix string
		Ok     bool
	}{
		{
			Path:   `/usr/local/go/src`,
			Prefix: `/usr/local/go/src`,
			Ok:     true,
		},
		{
			Path:   `/usr/local/go/src`,
			Prefix: `/usr/local/go`,
			Ok:     true,
		},
		{
			Path:   `/usr/local/go/src`,
			Prefix: `/usr//local//`,
			Ok:     true,
		},
		{
			Path:   `/usr/local/go/src`,
			Prefix: `/usr//locll//`,
			Ok:     false,
		},
		{
			Path:   `/usr/local/go/src`,
			Prefix: `/usr/local/go/src/bufio`,
			Ok:     false,
		},

		// Only works on Windows, TODO: Add build flags for tests.
		{
			Path:   `C:/usr/local/go/src`,
			Prefix: `C:\usr\local\go\src`,
			Ok:     false,
		},
	}
	for _, x := range tests {
		if ok := hasPrefix(x.Path, x.Prefix); ok != x.Ok {
			t.Errorf("HasPrefix (%+v): Exp (%v) Got (%v)", x, x.Ok, ok)
		}
	}
}

func TestIsInternal(t *testing.T) {
	var tests = []struct {
		Path string
		Ok   bool
	}{
		{
			Path: `/usr/local/go/internal`,
			Ok:   true,
		},
		{
			Path: `/usr/local/go/src`,
			Ok:   false,
		},
	}
	for _, x := range tests {
		if ok := isInternal(x.Path); ok != x.Ok {
			t.Errorf("IsInternal (%+v): Exp (%v) Got (%v)", x, x.Ok, ok)
		}
	}
}
