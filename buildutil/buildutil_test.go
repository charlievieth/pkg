// Copyright 2011 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package buildutil

import (
	"go/build"
	"reflect"
	"runtime"
	"testing"
)

// Copied from go/build/build_test.go
func TestMatch(t *testing.T) {
	ctxt := &build.Default
	what := "default"
	matchFn := func(tag string, want map[string]bool) {
		m := make(map[string]bool)
		if !match(ctxt, tag, m) {
			t.Errorf("%s context should match %s, does not", what, tag)
		}
		if !reflect.DeepEqual(m, want) {
			t.Errorf("%s tags = %v, want %v", tag, m, want)
		}
	}
	nomatch := func(tag string, want map[string]bool) {
		m := make(map[string]bool)
		if match(ctxt, tag, m) {
			t.Errorf("%s context should NOT match %s, does", what, tag)
		}
		if !reflect.DeepEqual(m, want) {
			t.Errorf("%s tags = %v, want %v", tag, m, want)
		}
	}

	matchFn(runtime.GOOS+","+runtime.GOARCH, map[string]bool{runtime.GOOS: true, runtime.GOARCH: true})
	matchFn(runtime.GOOS+","+runtime.GOARCH+",!foo", map[string]bool{runtime.GOOS: true, runtime.GOARCH: true, "foo": true})
	nomatch(runtime.GOOS+","+runtime.GOARCH+",foo", map[string]bool{runtime.GOOS: true, runtime.GOARCH: true, "foo": true})

	what = "modified"
	ctxt.BuildTags = []string{"foo"}
	matchFn(runtime.GOOS+","+runtime.GOARCH, map[string]bool{runtime.GOOS: true, runtime.GOARCH: true})
	matchFn(runtime.GOOS+","+runtime.GOARCH+",foo", map[string]bool{runtime.GOOS: true, runtime.GOARCH: true, "foo": true})
	nomatch(runtime.GOOS+","+runtime.GOARCH+",!foo", map[string]bool{runtime.GOOS: true, runtime.GOARCH: true, "foo": true})
	matchFn(runtime.GOOS+","+runtime.GOARCH+",!bar", map[string]bool{runtime.GOOS: true, runtime.GOARCH: true, "bar": true})
	nomatch(runtime.GOOS+","+runtime.GOARCH+",bar", map[string]bool{runtime.GOOS: true, runtime.GOARCH: true, "bar": true})
	nomatch("!", map[string]bool{})
}

// Copied from go/build/build_test.go
func TestShouldBuild(t *testing.T) {
	const file1 = "// +build tag1\n\n" +
		"package main\n"
	want1 := map[string]bool{"tag1": true}

	const file2 = "// +build cgo\n\n" +
		"// This package implements parsing of tags like\n" +
		"// +build tag1\n" +
		"package build"
	want2 := map[string]bool{"cgo": true}

	const file3 = "// Copyright The Go Authors.\n\n" +
		"package build\n\n" +
		"// shouldBuild checks tags given by lines of the form\n" +
		"// +build tag\n" +
		"func shouldBuild(content []byte)\n"
	want3 := map[string]bool{}

	ctx := &build.Context{BuildTags: []string{"tag1"}}
	m := map[string]bool{}
	if !shouldBuild(ctx, []byte(file1), m) {
		t.Errorf("shouldBuild(file1) = false, want true")
	}
	// Test exported wrapper around shouldBuild.
	if !ShouldBuild(ctx, []byte(file1), m) {
		t.Errorf("ShouldBuild(file1) = false, want true")
	}
	if !reflect.DeepEqual(m, want1) {
		t.Errorf("shoudBuild(file1) tags = %v, want %v", m, want1)
	}

	m = map[string]bool{}
	if shouldBuild(ctx, []byte(file2), m) {
		t.Errorf("shouldBuild(file2) = true, want false")
	}
	if ShouldBuild(ctx, []byte(file2), m) {
		t.Errorf("ShouldBuild(file2) = true, want false")
	}
	if !reflect.DeepEqual(m, want2) {
		t.Errorf("shoudBuild(file2) tags = %v, want %v", m, want2)
	}

	m = map[string]bool{}
	ctx = &build.Context{BuildTags: nil}
	if !shouldBuild(ctx, []byte(file3), m) {
		t.Errorf("shouldBuild(file3) = false, want true")
	}
	if !ShouldBuild(ctx, []byte(file3), m) {
		t.Errorf("ShouldBuild(file3) = false, want true")
	}
	if !reflect.DeepEqual(m, want3) {
		t.Errorf("shoudBuild(file3) tags = %v, want %v", m, want3)
	}
}

// Test that shouldBuild only reads the leading run of comments.
//
// The build package stops reading the file after imports are completed.
// This tests that shouldBuild does not include build tags that follow
// the "package" clause when passed a complete Go source file.
func TestShouldBuild_Full(t *testing.T) {
	const file1 = "// Copyright The Go Authors.\n\n" +
		"// +build tag1\n\n" + // Valid tag
		"// +build tag2\n" + // Bad tag (no following blank line)
		"package build\n\n" +
		"// +build tag3\n\n" + // Bad tag (after "package" statement)
		"import \"bytes\"\n\n" +
		"// shouldBuild checks tags given by lines of the form\n" +
		"func shouldBuild(content []byte) bool {\n" +
		"// +build tag4\n" + // Bad tag (after "package" statement)
		"\treturn bytes.Equal(content, []byte(\"content\")\n" +
		"}\n\n"
	want1 := map[string]bool{"tag1": true}

	const file2 = `
// Copyright The Go Authors.

// +build tag1
package build

// +build tag1

import "bytes"

// +build tag1

// shouldBuild checks tags given by lines of the form
// +build tag
func shouldBuild(content []byte) bool {

	// +build tag1

	return bytes.Equal(content, []byte("content")
}
`
	want2 := map[string]bool{}

	ctx := &build.Context{BuildTags: []string{"tag1"}}
	m := map[string]bool{}
	if !shouldBuild(ctx, []byte(file1), m) {
		t.Errorf("shouldBuild(file1) = false, want true")
	}
	if !reflect.DeepEqual(m, want1) {
		t.Errorf("shoudBuild(file1) tags = %v, want %v", m, want1)
	}

	m = map[string]bool{}
	if !shouldBuild(ctx, []byte(file2), m) {
		t.Errorf("shouldBuild(file2) = true, want false")
	}
	if !reflect.DeepEqual(m, want2) {
		t.Errorf("shoudBuild(file2) tags = %v, want %v", m, want2)
	}
}
