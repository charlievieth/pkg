package pkg2

// This file contains utilities for working with file paths and Go files.

import (
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"
)

var whitelistedExts = map[string]bool{
	".bash":        true,
	".c":           true,
	".cc":          true,
	".cpp":         true,
	".css":         true,
	".cxx":         true,
	".go":          true,
	".goc":         true,
	".h":           true,
	".hh":          true,
	".hpp":         true,
	".html":        true,
	".hxx":         true,
	".js":          true,
	".md":          true,
	".out":         true,
	".png":         true,
	".py":          true,
	".rb":          true,
	".s":           true,
	".sh":          true,
	".txt":         true,
	".xml":         true,
	"AUTHORS":      true,
	"CONTRIBUTORS": true,
	"LICENSE":      true,
	"Makefile":     true,
	"PATENTS":      true,
	"README":       true,
}

func isWhitelisted(filename string) bool {
	key := pathpkg.Ext(filename)
	if key == "" {
		key = filename
	}
	return whitelistedExts[key]
}

// Filenames ignored by dirtree.
var ignoredNames = map[string]bool{
	// Conventional name for directories containing test data.
	// Excluded from directory trees.
	"testdata": true,
}

func isIgnored(filename string) bool {
	return ignoredNames[pathpkg.Base(filename)]
}

func validName(s string) bool {
	return len(s) > 0 && s[0] != '_' && s[0] != '.'
}

// isPkgDir, returns if name is a possible package directory.
func isPkgDir(fi os.FileInfo) bool {
	return fi.IsDir() && validName(fi.Name())
}

func isGoFile(fi os.FileInfo) bool {
	name := fi.Name()
	return !fi.IsDir() && validName(name) &&
		strings.HasSuffix(name, ".go")
}

func isGoTestFile(fi os.FileInfo) bool {
	name := fi.Name()
	return !fi.IsDir() && validName(name) &&
		strings.HasSuffix(name, "_test.go")
}

func hasGoFiles(names []os.FileInfo) bool {
	for _, fi := range names {
		if isGoFile(fi) {
			return true
		}
	}
	return false
}

func isInternal(p string) bool {
	return pathpkg.Base(p) == "internal"
}

// trimPathPrefix, remove the prefix from path s.
func trimPathPrefix(s, prefix string) string {
	if hasRoot(s, prefix) {
		return strings.TrimLeft(s[len(prefix):], "/")
	}
	return s
}

// hasRoot, returns if path is inside the directory tree rooted at root.
// Should be used for internal paths (i.e. paths we know to clean).
func hasRoot(path, root string) bool {
	// TODO: 'hasRoot' is a bad name, merge with 'hasPrefix'
	return len(path) >= len(root) && path[0:len(root)] == root
}

// hasPrefix, returns if the path is inside the directory tree rooted at root.
// Unlike hasRoot the path is not assumed to be clean.  The prefix must be
// clean.  Use when matching external strings.
func hasPrefix(path, prefix string) bool {
	// TODO: Remove if unused.
	if len(path) < len(prefix) {
		return false
	}
	if path[0:len(prefix)] == prefix {
		return true
	}
	return hasRoot(clean(path), prefix)
}

func clean(path string) string {
	return pathpkg.Clean(filepath.ToSlash(path))
}
