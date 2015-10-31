package pkg

import (
	"go/parser"
	"go/token"
	"io/ioutil"
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

const maxOpenFiles = 200

var fsOpenGate = make(chan struct{}, maxOpenFiles)

func readdirnames(path string) ([]string, error) {
	fsOpenGate <- struct{}{}
	defer func() { <-fsOpenGate }()
	f, err := os.Open(path)
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

// completeDirnames, reads the dirnames for path if names are nil.
func completeDirnames(path string, names []string) ([]string, error) {
	// TODO: rename
	if names != nil {
		return names, nil
	}
	return readdirnames(path)
}

func readdirmap(path string) (map[string]bool, error) {
	s, err := readdirnames(path)
	if err != nil {
		return nil, err
	}
	m := make(map[string]bool, len(s))
	for i := 0; i < len(s); i++ {
		m[s[i]] = true
	}
	return m, nil
}

func readFile(path string) ([]byte, error) {
	fsOpenGate <- struct{}{}
	defer func() { <-fsOpenGate }()
	return ioutil.ReadFile(path)
}

func parseFileName(path string, fset *token.FileSet) (name string, ok bool) {
	src, err := readFile(path)
	if err != nil {
		return "", false
	}
	af, _ := parser.ParseFile(fset, path, src, parser.PackageClauseOnly)
	if af != nil && af.Name != nil {
		name = af.Name.Name
	}
	return name, name != ""
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

func hasGoFiles(names []string) bool {
	for _, n := range names {
		if strings.HasSuffix(n, ".go") {
			return true
		}
	}
	return false
}

func sameFile(fi1, fi2 os.FileInfo) bool {
	if fi1 == nil {
		if fi2 == nil {
			return true
		}
		return false
	}
	return fi1.ModTime() == fi2.ModTime() &&
		fi1.Size() == fi2.Size() &&
		fi1.Name() == fi2.Name() &&
		fi1.IsDir() == fi2.IsDir()
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

func trimPathPrefix(s, prefix string) string {
	if sameRoot(s, prefix) {
		return strings.TrimPrefix(s[len(prefix):], string(filepath.Separator))
	}
	return s
}
