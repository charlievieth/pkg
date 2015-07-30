package pkg

import (
	"fmt"
	"go/build"
	"go/parser"
	"go/token"
	"os"
	"path"
	"time"
)

var (
	_ = time.ANSIC
	_ = fmt.Sprint("")
)

type File struct {
	Name string
	Info os.FileInfo
}

type Package struct {
	Name     string
	Internal bool
	Files    map[string]*File
}

type Config struct {
	Context build.Context
}

// Conventional name for directories containing test data.
// Excluded from directory trees.
//
const testdataDirName = "testdata"

type Directory struct {
	Path     string // directory path
	Name     string // directory name
	PkgName  string // Go pkg name
	HasPkg   bool   // has Go pkg
	Internal bool   // Internal Go pkg
	Info     os.FileInfo
	Dirs     map[string]*Directory
	Depth    int
}

func (c *Config) newDirTree(fset *token.FileSet, filepath, name string,
	depth int, internal bool) *Directory {

	list, err := readdirnames(filepath)
	if err != nil {
		return nil // Change
	}
	fi, err := os.Stat(filepath)
	if err != nil {
		return nil
	}
	if !internal && name == "internal" {
		internal = true
	}
	var (
		hasPkgFiles bool
		pkgName     string
		dirchs      []chan *Directory
	)
	for _, d := range list {
		switch {
		case isPkgDir(d):
			ch := make(chan *Directory)
			dirchs = append(dirchs, ch)
			go func(d string) {
				ch <- c.newDirTree(fset, path.Join(filepath, d), d, depth+1,
					internal)
			}(d)
		case isGoFile(d):
			if pkgName == "" {
				af, err := parser.ParseFile(fset, path.Join(filepath, d), nil,
					parser.PackageClauseOnly)
				if err == nil && af.Name != nil {
					hasPkgFiles = true
					pkgName = af.Name.Name
				}
			}
		}
	}
	dirs := make(map[string]*Directory)
	for _, ch := range dirchs {
		if d := <-ch; d != nil {
			dirs[d.Name] = d
		}
	}
	if !hasPkgFiles && len(dirs) == 0 {
		return nil
	}
	return &Directory{
		Path:     filepath,
		Name:     name,
		PkgName:  pkgName,
		HasPkg:   hasPkgFiles,
		Internal: internal,
		Info:     fi,
		Dirs:     dirs,
		Depth:    depth,
	}
}

func (c *Config) NewDirectory(root string) *Directory {
	d, err := os.Stat(root)
	if err != nil || !d.IsDir() {
		return nil
	}
	internal := path.Base(root) == "internal"
	fset := token.NewFileSet()
	return c.newDirTree(fset, root, d.Name(), 0, internal)
}
