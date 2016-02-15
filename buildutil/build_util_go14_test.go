// Copyright 2011 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build go1.4

package buildutil

import (
	"go/build"
	"io"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type readNopCloser struct {
	io.Reader
}

func (r readNopCloser) Close() error {
	return nil
}

var (
	ctxtP9      = build.Context{GOARCH: "arm", GOOS: "plan9"}
	ctxtAndroid = build.Context{GOARCH: "arm", GOOS: "android"}
)

var matchFileTests = []struct {
	ctxt  build.Context
	name  string
	data  string
	match bool
}{
	{ctxtP9, "foo_arm.go", "", true},
	{ctxtP9, "foo1_arm.go", "// +build linux\n\npackage main\n", false},
	{ctxtP9, "foo_darwin.go", "", false},
	{ctxtP9, "foo.go", "", true},
	{ctxtP9, "foo1.go", "// +build linux\n\npackage main\n", false},
	{ctxtP9, "foo.badsuffix", "", false},
	{ctxtAndroid, "foo_linux.go", "", true},
	{ctxtAndroid, "foo_android.go", "", true},
	{ctxtAndroid, "foo_plan9.go", "", false},
	{ctxtAndroid, "android.go", "", true},
	{ctxtAndroid, "plan9.go", "", true},
	{ctxtAndroid, "plan9_test.go", "", true},
	{ctxtAndroid, "arm.s", "", true},
	{ctxtAndroid, "amd64.s", "", true},
}

func TestMatchFile(t *testing.T) {
	for _, tt := range matchFileTests {
		ctxt := tt.ctxt
		ctxt.OpenFile = func(path string) (r io.ReadCloser, err error) {
			if path != "x+"+tt.name {
				t.Fatalf("OpenFile asked for %q, expected %q", path, "x+"+tt.name)
			}
			return &readNopCloser{strings.NewReader(tt.data)}, nil
		}
		ctxt.JoinPath = func(elem ...string) string {
			return strings.Join(elem, "+")
		}
		match, err := MatchFile(&ctxt, "x", tt.name)
		if match != tt.match || err != nil {
			t.Fatalf("MatchFile(%q) = %v, %v, want %v, nil", tt.name, match, err, tt.match)
		}
	}
}

func TestImportCmd(t *testing.T) {
	if runtime.GOOS == "darwin" {
		switch runtime.GOARCH {
		case "arm", "arm64":
			t.Skipf("skipping on %s/%s, no valid GOROOT", runtime.GOOS, runtime.GOARCH)
		}
	}

	p, err := Import(&Default, "cmd/internal/objfile", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(filepath.ToSlash(p.Dir), "src/cmd/internal/objfile") {
		t.Fatalf("Import cmd/internal/objfile returned Dir=%q, want %q", filepath.ToSlash(p.Dir), ".../src/cmd/internal/objfile")
	}
}
