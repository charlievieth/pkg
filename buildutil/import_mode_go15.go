// +build !go1.6

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

	// If ImportComment is set, parse import comments on package statements.
	// Import returns an error if it finds a comment it cannot understand
	// or finds conflicting comments in multiple source files.
	// See golang.org/s/go14customimport for more information.
	ImportComment = build.ImportComment

	// Backported from Go 1.6
	//
	// By default, Import searches vendor directories
	// that apply in the given source directory before searching
	// the GOROOT and GOPATH roots.
	// If an Import finds and returns a package using a vendor
	// directory, the resulting ImportPath is the complete path
	// to the package, including the path elements leading up
	// to and including "vendor".
	// For example, if Import("y", "x/subdir", 0) finds
	// "x/vendor/y", the returned package's ImportPath is "x/vendor/y",
	// not plain "y".
	// See golang.org/s/go15vendor for more information.
	//
	// Setting IgnoreVendor ignores vendor directories.
	IgnoreVendor = build.ImportComment << 1
)
