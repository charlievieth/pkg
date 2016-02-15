// Copyright 2011 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build go1.5,!go1.6

package buildutil

import (
	"bytes"
	"fmt"
	"go/build"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// isMacro reports whether p is a package dependency macro
// (uppercase name).
func isMacro(p string) bool {
	return 'A' <= p[0] && p[0] <= 'Z'
}

func allowed(pkg string) map[string]bool {
	m := map[string]bool{}
	var allow func(string)
	allow = func(p string) {
		if m[p] {
			return
		}
		m[p] = true // set even for macros, to avoid loop on cycle

		// Upper-case names are macro-expanded.
		if isMacro(p) {
			for _, pp := range pkgDeps[p] {
				allow(pp)
			}
		}
	}
	for _, pp := range pkgDeps[pkg] {
		allow(pp)
	}
	return m
}

var bools = []bool{false, true}
var geese = []string{"android", "darwin", "dragonfly", "freebsd", "linux", "nacl", "netbsd", "openbsd", "plan9", "solaris", "windows"}
var goarches = []string{"386", "amd64", "arm"}

type osPkg struct {
	goos, pkg string
}

// allowedErrors are the operating systems and packages known to contain errors
// (currently just "no Go source files")
var allowedErrors = map[osPkg]bool{
	osPkg{"windows", "log/syslog"}: true,
	osPkg{"plan9", "log/syslog"}:   true,
}

// listStdPkgs returns the same list of packages as "go list std".
func listStdPkgs(goroot string) ([]string, error) {
	// Based on cmd/go's matchPackages function.
	var pkgs []string

	src := filepath.Join(goroot, "src") + string(filepath.Separator)
	walkFn := func(path string, fi os.FileInfo, err error) error {
		if err != nil || !fi.IsDir() || path == src {
			return nil
		}

		base := filepath.Base(path)
		if strings.HasPrefix(base, ".") || strings.HasPrefix(base, "_") || base == "testdata" {
			return filepath.SkipDir
		}

		name := filepath.ToSlash(path[len(src):])
		if name == "builtin" || name == "cmd" || strings.Contains(name, ".") {
			return filepath.SkipDir
		}

		pkgs = append(pkgs, name)
		return nil
	}
	if err := filepath.Walk(src, walkFn); err != nil {
		return nil, err
	}
	return pkgs, nil
}

func TestDependencies(t *testing.T) {
	iOS := runtime.GOOS == "darwin" && (runtime.GOARCH == "arm" || runtime.GOARCH == "arm64")
	if runtime.GOOS == "nacl" || iOS {
		// Tests run in a limited file system and we do not
		// provide access to every source file.
		t.Skipf("skipping on %s/%s, missing full GOROOT", runtime.GOOS, runtime.GOARCH)
	}

	ctxt := build.Default
	all, err := listStdPkgs(ctxt.GOROOT)
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(all)

	for _, pkg := range all {
		imports, err := findImports(pkg)
		if err != nil {
			t.Error(err)
			continue
		}
		ok := allowed(pkg)
		var bad []string
		for _, imp := range imports {
			if !ok[imp] {
				bad = append(bad, imp)
			}
		}
		if bad != nil {
			t.Errorf("unexpected dependency: %s imports %v", pkg, bad)
		}
	}
}

var buildIgnore = []byte("\n// +build ignore")

func findImports(pkg string) ([]string, error) {
	dir := filepath.Join(build.Default.GOROOT, "src", pkg)
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var imports []string
	var haveImport = map[string]bool{}
	for _, file := range files {
		name := file.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := os.Open(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		var imp []string
		data, err := readImports(f, false, &imp)
		f.Close()
		if err != nil {
			return nil, fmt.Errorf("reading %v: %v", name, err)
		}
		if bytes.Contains(data, buildIgnore) {
			continue
		}
		for _, quoted := range imp {
			path, err := strconv.Unquote(quoted)
			if err != nil {
				continue
			}
			if !haveImport[path] {
				haveImport[path] = true
				imports = append(imports, path)
			}
		}
	}
	sort.Strings(imports)
	return imports, nil
}

var (
	expandSrcDirPath = filepath.Join(string(filepath.Separator)+"projects", "src", "add")
)

var expandSrcDirTests = []struct {
	input, expected string
}{
	{"-L ${SRCDIR}/libs -ladd", "-L /projects/src/add/libs -ladd"},
	{"${SRCDIR}/add_linux_386.a -pthread -lstdc++", "/projects/src/add/add_linux_386.a -pthread -lstdc++"},
	{"Nothing to expand here!", "Nothing to expand here!"},
	{"$", "$"},
	{"$$", "$$"},
	{"${", "${"},
	{"$}", "$}"},
	{"$FOO ${BAR}", "$FOO ${BAR}"},
	{"Find me the $SRCDIRECTORY.", "Find me the $SRCDIRECTORY."},
	{"$SRCDIR is missing braces", "$SRCDIR is missing braces"},
}

func TestExpandSrcDir(t *testing.T) {
	for _, test := range expandSrcDirTests {
		output, _ := expandSrcDir(test.input, expandSrcDirPath)
		if output != test.expected {
			t.Errorf("%q expands to %q with SRCDIR=%q when %q is expected", test.input, output, expandSrcDirPath, test.expected)
		} else {
			t.Logf("%q expands to %q with SRCDIR=%q", test.input, output, expandSrcDirPath)
		}
	}
}

func TestShellSafety(t *testing.T) {
	tests := []struct {
		input, srcdir, expected string
		result                  bool
	}{
		{"-I${SRCDIR}/../include", "/projects/src/issue 11868", "-I/projects/src/issue 11868/../include", true},
		{"-X${SRCDIR}/1,${SRCDIR}/2", "/projects/src/issue 11868", "-X/projects/src/issue 11868/1,/projects/src/issue 11868/2", true},
		{"-I/tmp -I/tmp", "/tmp2", "-I/tmp -I/tmp", false},
		{"-I/tmp", "/tmp/[0]", "-I/tmp", true},
		{"-I${SRCDIR}/dir", "/tmp/[0]", "-I/tmp/[0]/dir", false},
	}
	for _, test := range tests {
		output, ok := expandSrcDir(test.input, test.srcdir)
		if ok != test.result {
			t.Errorf("Expected %t while %q expands to %q with SRCDIR=%q; got %t", test.result, test.input, output, test.srcdir, ok)
		}
		if output != test.expected {
			t.Errorf("Expected %q while %q expands with SRCDIR=%q; got %q", test.expected, test.input, test.srcdir, output)
		}
	}
}
