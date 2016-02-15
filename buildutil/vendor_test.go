// +build go1.5

// Go 1.5 specific vendor tests.

package buildutil

import (
	"go/build"
	"path/filepath"
	"strings"
	"testing"
)

func TestImportVendor(t *testing.T) {
	path := "vendortest"
	srcDir := CurrentWorkingDirectory

	// Make sure the 'vendortest' package does not exist in $GOPATH/src.
	// This also tests the IgnoreVendor flag.
	if _, err := Import(&Default, path, srcDir, IgnoreVendor); err == nil {
		t.Fatalf("found made-up package %s in %s directory: %v", path, srcDir, err)
	}

	p, err := Import(&Default, path, srcDir, 0)
	if err != nil {
		t.Fatalf("cannot find vendored %s from %s directory: %v", path, srcDir, err)
	}
	want := filepath.Join(CurrentImportPath, "vendor", path)
	if p.ImportPath != want {
		t.Fatalf("Import succeeded but found %q, want %q", p.ImportPath, want)
	}
}

func TestImportVendorFailure(t *testing.T) {
	ctxt := Default
	ctxt.GOROOT = ""
	p, err := Import(&ctxt, "x.com/y/z", CurrentWorkingDirectory, 0)
	if err == nil {
		t.Fatalf("found made-up package x.com/y/z in %s", p.Dir)
	}

	e := err.Error()
	if !strings.Contains(e, " (vendor tree)") {
		t.Fatalf("error on failed import does not mention %s directory:\n%s",
			filepath.Join(CurrentImportPath, "vendor"), e)
	}
}

func TestImportVendorParentNoGoFailure(t *testing.T) {
	ctxt := Default
	// This import should fail because the database directory has no source code.
	p, err := Import(&ctxt, "database", CurrentWorkingDirectory, 0)
	if err == nil {
		t.Fatalf("found empty parent in %s", p.Dir)
	}
	if _, ok := err.(*build.NoGoError); !ok {
		t.Fatalf("expected build.NoGoError: %#v", err)
	}
}
