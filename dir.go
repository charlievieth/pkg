package pkg

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var whitelisted = map[string]bool{
	".bash":        true,
	".c":           true,
	".cc":          true,
	".cpp":         true,
	".cxx":         true,
	".css":         true,
	".go":          true,
	".goc":         true,
	".h":           true,
	".hh":          true,
	".hpp":         true,
	".hxx":         true,
	".html":        true,
	".js":          true,
	".out":         true,
	".py":          true,
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
	key := filepath.Ext(filename)
	if key == "" {
		key = filename
	}
	return whitelisted[key]
}

func readdirnames(name string) ([]string, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	names, err := f.Readdirnames(-1)
	f.Close()
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	return names, nil
}

func readdirmap(name string) (map[string]bool, error) {
	s, err := readdirnames(name)
	if err != nil {
		return nil, err
	}
	m := make(map[string]bool, len(s))
	for i := 0; i < len(s); i++ {
		m[s[i]] = true
	}
	return m, nil
}

// isPkgDir, returns if name is a possible package directory.
func isPkgDir(name string) bool {
	return validName(name) && !isWhitelisted(name)
}

func isDir(name string) bool {
	fs, err := os.Stat(name)
	return err == nil && fs.IsDir()
}

func isGoFile(name string) bool {
	return validName(name) && filepath.Ext(name) == ".go"
}

func isGoTestFile(name string) bool {
	return validName(name) && strings.HasSuffix(name, "_test.go")
}

func validName(s string) bool {
	return len(s) > 0 && s[0] != '_' && s[0] != '.'
}

func sameFile(fs1, fs2 os.FileInfo) bool {
	return fs1.ModTime() == fs2.ModTime() &&
		fs1.Size() == fs2.Size() &&
		fs1.Mode() == fs2.Mode() &&
		fs1.Name() == fs2.Name() &&
		fs1.IsDir() == fs2.IsDir()
}

// filepathDir, returns the directory of path.  If path is a file the parent
// directory is returned.  If path is a directory it is cleaned and returned.
func filepathDir(path string) string {
	path = filepath.Clean(path)
	if path == "" {
		return path
	}
	fi, err := os.Stat(path)
	if err != nil || fi.IsDir() {
		return path
	}
	return filepath.Dir(path)
}

type FilterFunc func(string) bool

func FilterList(list []string, fn FilterFunc) []string {
	n := 0
	for i := 0; i < len(list); i++ {
		if fn(list[i]) {
			list[n] = list[i]
			n++
		}
	}
	return list[:n]
}
