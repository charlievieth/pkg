package pkg

import (
	"fmt"
	"go/build"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

var (
	_ = time.ANSIC
	_ = fmt.Sprint("")
)

const pathSeparator = string(os.PathSeparator)

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

type Corpus struct {
	ctxt     build.Context
	dirs     map[string]*Directory
	srcDirs  []string
	MaxDepth int
}

type treeBuilder struct {
	c        *Corpus
	maxDepth int
}

func NewCorpus() *Corpus {
	ctxt := build.Default
	ctxt.GOPATH = os.Getenv("GOPATH")
	ctxt.GOROOT = runtime.GOROOT()
	srcDirs := ctxt.SrcDirs()
	dirs := make([]*Directory, len(srcDirs))
	// for i := 0; i < len(srcDirs); i++ {
	// 	if d :=  {

	// 	}
	// }
	_ = dirs
	return nil
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
	// GoFiles  map[string]struct{}
}

func (c *Corpus) treeBuilder() *treeBuilder {
	return &treeBuilder{c: c, maxDepth: c.MaxDepth}
}

func (c *Corpus) updateEnv() {
	path := os.Getenv("GOPATH")
	root := runtime.GOROOT()
	if path != c.ctxt.GOPATH || root != c.ctxt.GOROOT {
		c.ctxt.GOPATH = path
		c.ctxt.GOROOT = root
		c.srcDirs = c.ctxt.SrcDirs()
		for _, d := range c.srcDirs {
			if _, ok := c.dirs[d]; !ok {
				delete(c.dirs, d)
			}
		}
	}
}

func (c *Corpus) Update(dir *Directory) {
	c.updateEnv()
	t := c.treeBuilder()
	for _, path := range c.srcDirs {
		fset := token.NewFileSet()
		if _, ok := c.dirs[path]; ok {
			c.dirs[path] = t.updateDirTree(c.dirs[path], fset)
		} else {
			c.dirs[path] = t.newDirTree(fset, path, filepath.Base(path), 0, false)
		}
	}
}

func (t *treeBuilder) updateDirTree(dir *Directory, fset *token.FileSet) *Directory {
	fi, err := os.Stat(dir.Path)
	if err != nil {
		return nil
	}
	var dirchs []chan *Directory
	switch {
	case !sameFile(fi, dir.Info):
		dir.Info = fi
		list, err := readdirmap(dir.Path)
		if err != nil {
			return nil
		}
		hasPkgFiles := false
		for d := range list {
			switch {
			case isPkgDir(d):
				if _, ok := dir.Dirs[d]; !ok {
					ch := make(chan *Directory)
					dirchs = append(dirchs, ch)
					go func(d string) {
						ch <- t.newDirTree(fset, filepath.Join(dir.Path, d), d,
							dir.Depth+1, dir.Internal)
					}(d)
				}
			case isGoFile(d):
				hasPkgFiles = true
				if !dir.HasPkg && dir.PkgName == "" && t.c.matchFile(dir.Path, d) {
					name, ok := includePkg(filepath.Join(dir.Path, d), fset)
					if ok {
						dir.HasPkg = true
						dir.PkgName = name
					}
				}
			}
		}
		for n := range dir.Dirs {
			if !list[n] {
				delete(dir.Dirs, n)
			}
		}
		if !hasPkgFiles && dir.HasPkg {
			dir.HasPkg = false
			dir.PkgName = ""
		}
		if len(dir.Dirs) == 0 && !dir.HasPkg {
			return nil
		}
	default:
		for _, d := range dir.Dirs {
			ch := make(chan *Directory)
			dirchs = append(dirchs, ch)
			go func(d *Directory) {
				ch <- t.updateDirTree(d, fset)
			}(d)
		}
	}
	for _, ch := range dirchs {
		if d := <-ch; d != nil {
			if dir.Dirs == nil {
				dir.Dirs = make(map[string]*Directory)
			}
			dir.Dirs[d.Name] = d
		}
	}
	return dir
}

func sameFile(fs1, fs2 os.FileInfo) bool {
	return fs1.ModTime() == fs2.ModTime() &&
		fs1.Size() == fs2.Size() &&
		fs1.Mode() == fs2.Mode() &&
		fs1.Name() == fs2.Name() &&
		fs1.IsDir() == fs2.IsDir()
}

func (t *treeBuilder) newDirTree(fset *token.FileSet, path, name string,
	depth int, internal bool) *Directory {
	if depth >= t.maxDepth {
		return &Directory{
			Depth: depth,
			Path:  path,
			Name:  name,
		}
	}
	list, err := readdirnames(path)
	if err != nil {
		return nil // Change
	}
	fi, err := os.Stat(path)
	if err != nil {
		return nil
	}
	if !internal && isInternal(name) {
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
				ch <- t.newDirTree(fset, filepath.Join(path, d), d, depth+1,
					internal)
			}(d)
		case isGoFile(d):
			if pkgName == "" && t.c.matchFile(path, d) {
				name, ok := includePkg(filepath.Join(path, d), fset)
				if ok {
					hasPkgFiles = true
					pkgName = name
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
		Path:     path,
		Name:     name,
		PkgName:  pkgName,
		HasPkg:   hasPkgFiles,
		Internal: internal,
		Info:     fi,
		Dirs:     dirs,
		Depth:    depth,
	}
}

func (c *Corpus) matchFile(dir, name string) bool {
	ok, err := c.ctxt.MatchFile(dir, name)
	return ok && err == nil
}

func includePkg(path string, fset *token.FileSet) (string, bool) {
	af, err := parser.ParseFile(fset, path, nil, parser.PackageClauseOnly)
	if err == nil && af.Name != nil && af.Name.Name != "main" {
		return af.Name.Name, true
	}
	return "", false
}

// TODO: make private when done testing
func (c *Corpus) NewDirectory(root string) *Directory {
	// root = filepath.Clean(root)
	// d, err := os.Stat(root)
	// if err != nil || !d.IsDir() {
	// 	return nil
	// }
	// return c.newDirTree(token.NewFileSet(), root, d.Name(), 0, isInternal(root))
	return nil
}

func isInternal(p string) bool {
	return filepath.Base(p) == "internal"
}

func splitPath(p string) []string {
	p = strings.TrimPrefix(p, pathSeparator)
	if p == "" {
		return nil
	}
	return strings.Split(p, pathSeparator)
}

func (dir *Directory) Lookup(filepath string) *Directory {
	d := splitPath(dir.Path)
	p := splitPath(filepath)
	i := 0
	for i < len(d) {
		if i >= len(p) || d[i] != p[i] {
			return nil
		}
		i++
	}
	for dir != nil && i < len(p) {
		dir = dir.Dirs[p[i]]
		i++
	}
	return dir
}

func (c *Corpus) ListImports(path string) []string {
	if c.dirs == nil || len(c.dirs) == 0 {
		return nil
	}
	if fi, err := os.Stat(path); err == nil {
		if fi.IsDir() {
			path = filepath.Dir(path)
		} else {
			path = filepath.Clean(path)
		}
	}
	list := make([]string, 0, 1024)
	for _, d := range c.dirs {
		d.listPkgs(path, &list)
	}
	sort.Strings(list)
	return list
}

func dirPath(p string) string {
	if fi, err := os.Stat(p); err == nil {
		if fi.IsDir() {
			return filepath.Dir(p)
		}
		return filepath.Clean(p)
	}
	v := filepath.VolumeName(p)
	for p != v {
		p = filepath.Dir(p)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return p
}

func (dir *Directory) ImportList(path string) []string {
	if fi, err := os.Stat(path); err == nil {
		if fi.IsDir() {
			path = filepath.Dir(path)
		} else {
			path = filepath.Clean(path)
		}
	}
	list := make([]string, 0, 1024)
	dir.listPkgs(path, &list)
	sort.Strings(list)
	return list
}

func (dir *Directory) listPkgs(path string, list *[]string) {
	if dir.Internal && !filepath.HasPrefix(dir.Path, path) {
		return
	}
	*list = append(*list, dir.Path)
	for _, d := range dir.Dirs {
		d.listPkgs(path, list)
	}
}

func listDirs(dir *Directory, list *[]string, path string) {
	*list = append(*list, dir.Path)
	if dir.Dirs != nil {
		for _, d := range dir.Dirs {
			listDirs(d, list, path)
		}
	}
}
