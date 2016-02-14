// +build go1.6

// Copyright 2011 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package buildutil

import "go/build"

// Import returns details about the Go package named by the import path,
// interpreting local import paths relative to the srcDir directory.
// If the path is a local import path naming a package that can be imported
// using a standard import path, the returned package will set p.ImportPath
// to that path.
//
// In the directory containing the package, .go, .c, .h, and .s files are
// considered part of the package except for:
//
//	- .go files in package documentation
//	- files starting with _ or . (likely editor temporary files)
//	- files with build constraints not satisfied by the context
//
// If an error occurs, Import returns a non-nil error and a non-nil
// *Package containing partial information.
//
func Import(bc *build.Context, path string, srcDir string, mode build.ImportMode) (*build.Package, error) {
	if bc != nil {
		return bc.Import(path, srcDir, mode)
	}
	return build.Default.Import(path, srcDir, mode)
}

// ImportDir is like Import but processes the Go package found in
// the named directory.
func ImportDir(bc *build.Context, dir string, mode build.ImportMode) (*build.Package, error) {
	return Import(bc, ".", dir, mode)
}