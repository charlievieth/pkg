package pkg

import (
	"go/build"
	"runtime"
	"testing"
)

func TestIsInstalled(t *testing.T) {
	c := &Corpus{
		ctxt: NewContext(&build.Default, 0),
	}
	x := PackageIndex{c: c}

	var tests = []struct {
		p   Package
		exp bool
	}{
		// Test Packages
		{
			Package{
				Root:       runtime.GOROOT(),
				Name:       "testing",
				ImportPath: "testing",
			},
			true,
		},
		// Test invalid package
		{
			Package{
				Root:       runtime.GOROOT(),
				Name:       "INVALID",
				ImportPath: "INVALID",
			},
			false,
		},
		// Test Commands
		{
			Package{
				Root:       runtime.GOROOT(),
				Name:       "main",
				ImportPath: "cmd/go",
			},
			true,
		},
	}
	for _, test := range tests {
		p := &test.p
		if x.isInstalled(p) != test.exp {
			t.Fatalf("PackageIndex.isInstalled: name (%s) import path (%s) root (%s) expected: %v",
				p.Name, p.ImportPath, p.Root, test.exp)
		}
	}
}

func TestLookup(t *testing.T) {
	c := &Corpus{
		ctxt: NewContext(&build.Default, 0),
	}
	x := PackageIndex{c: c}

	pkg := &Package{
		Dir:        "/usr/local/go/src/bufio",
		Name:       "bufio",
		ImportPath: "bufio",
		Root:       "/usr/local/go",
		SrcRoot:    "/usr/local/go/src",
		Goroot:     true,
	}
	x.addPackage(pkg)

	if _, ok := x.lookupPath(pkg.Dir); !ok {
		t.Errorf("PackageIndex lookupPath: (%+v)\n", pkg)
	}
	if _, ok := x.lookup(pkg.SrcRoot, pkg.ImportPath); !ok {
		t.Errorf("PackageIndex lookup: (%+v)\n", pkg)
	}
}
