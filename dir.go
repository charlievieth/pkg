package pkg

import (
	"os"
	"path"
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
	key := path.Ext(filename)
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
	defer f.Close()
	return f.Readdirnames(-1)
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

func isPkgDir(name string) bool {
	return validName(name) && !isWhitelisted(name)
}

func isDir(name string) bool {
	fs, err := os.Stat(name)
	return err == nil && fs.IsDir()
}

func isGoFile(name string) bool {
	return validName(name) && path.Ext(name) == ".go"
}

func validName(s string) bool {
	return len(s) > 0 && s[0] != '_' && s[0] != '.'
}
