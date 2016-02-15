// Copyright 2011 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build go1.5

package buildutil

import (
	"go/build"
	"path/filepath"
	"reflect"
	"testing"
)

// It appears that before version 1.5 the build package incorrectly swapped
// the Packages and Files fields when constructing a MultiplePackageError.
func TestMultiplePackageImport(t *testing.T) {
	_, err := Import(&Default, ".", "testdata/multi", 0)
	mpe, ok := err.(*build.MultiplePackageError)
	if !ok {
		t.Fatal(`Import("testdata/multi") did not return MultiplePackageError.`)
	}
	want := &build.MultiplePackageError{
		Dir:      filepath.FromSlash("testdata/multi"),
		Packages: []string{"main", "test_package"},
		Files:    []string{"file.go", "file_appengine.go"},
	}
	if !reflect.DeepEqual(mpe, want) {
		t.Errorf("got %#v; want %#v", mpe, want)
	}
}
