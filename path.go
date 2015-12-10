package pkg

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

// isIgnored, returns if the filename should be ignored.
func isIgnored(filename string) bool {
	return ignoredNames[pathpkg.Base(filename)]
}

// validName returns if s does not start with a '.' or '_'.
func validName(s string) bool {
	return len(s) > 0 && s[0] != '_' && s[0] != '.'
}

// isPkgDir, returns if name is a possible package directory.
func isPkgDir(fi os.FileInfo) bool {
	return fi.IsDir() && validName(fi.Name())
}

// isGoFile returns if the file described by fi may be a Go source file.
func isGoFile(fi os.FileInfo) bool {
	name := fi.Name()
	return !fi.IsDir() && validName(name) &&
		strings.HasSuffix(name, ".go")
}

// isGoFile returns if the file described by fi may be a Go test file.
func isGoTestFile(fi os.FileInfo) bool {
	name := fi.Name()
	return !fi.IsDir() && validName(name) &&
		strings.HasSuffix(name, "_test.go")
}

// hasGoFiles returns if any of the names may be a Go source file.
func hasGoFiles(names []os.FileInfo) bool {
	for _, fi := range names {
		if isGoFile(fi) {
			return true
		}
	}
	return false
}

// isInternal returns if the base of path equals 'internal'.  Used for
// identifying internal Go package directories.
func isInternal(path string) bool {
	return pathpkg.Base(path) == "internal"
}

// trimPathPrefix, remove the prefix from path s.
func trimPathPrefix(s, prefix string) string {
	if hasRoot(s, prefix) {
		return strings.TrimLeft(s[len(prefix):], "/")
	}
	return s
}

// hasRoot, returns if path is inside the directory tree rooted at root.
// Should be used for internal paths (i.e. clean slash-separated paths).
func hasRoot(path, root string) bool {
	// TODO: 'hasRoot' is a bad name, merge with 'hasPrefix'
	return len(path) >= len(root) && path[0:len(root)] == root
}

// hasPrefix, returns if the path is inside the directory tree rooted at root.
// Unlike hasRoot the path is not assumed to be clean.  The prefix must be
// clean.  Use when matching external strings.
func hasPrefix(path, prefix string) bool {
	if strings.HasPrefix(path, prefix) {
		return true
	}
	if len(path) < len(prefix) {
		return false
	}
	if strings.ContainsRune(prefix, os.PathSeparator) {
		return strings.HasPrefix(path, clean(prefix))
	}
	return false
}

// clean, converts OS specific separators to slashes and cleans path.
func clean(path string) string {
	return pathpkg.Clean(filepath.ToSlash(path))
}
