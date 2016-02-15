// Copyright 2011 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build !go1.4

// build.ImportMode for Go versions 1.4 and below (no support vendoring).

package buildutil

import "go/build"

const (
	// If FindOnly is set, Import stops after locating the directory
	// that should contain the sources for a package.  It does not
	// read any files in the directory.
	FindOnly = build.FindOnly

	// If AllowBinary is set, Import can be satisfied by a compiled
	// package object without corresponding sources.
	AllowBinary = build.AllowBinary
)

// SetIgnoreVendor sets the IgnoreVendor bits for build.ImportMode mode if the
// "GO15VENDOREXPERIMENT" environment variable is "1" and returns the updated
// build.ImportMode.
//
// For Go version 1.5 only.  All other Go versions return the mode unmodified.
func SetIgnoreVendor(mode build.ImportMode) build.ImportMode { return mode }
