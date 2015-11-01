package pkg

// TODO:
// 	- Consider removing Corupus.MaxDepth
//  - Consider making pkg selection version aware

import (
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type treeBuilder struct {
	c        *Corpus
	maxDepth int
}

func newTreeBuilder(c *Corpus) *treeBuilder {
	depth := c.MaxDepth
	if depth <= 0 {
		depth = 512
	}
	return &treeBuilder{c: c, maxDepth: depth}
}

// Conventional name for directories containing test data.
// Excluded from directory trees.
//
const testdataDirName = "testdata"

type Directory struct {
	Pkg *Package

	Path     string // directory path
	Name     string // directory name
	PkgName  string // Go pkg name
	HasPkg   bool   // has Go pkg
	Internal bool   // Internal Go pkg
	Info     os.FileInfo
	Dirs     map[string]*Directory
	Depth    int
}

func (d *Directory) removeNotFound(found []string) {
	sort.Strings(found)
	for n := range d.Dirs {
		if sort.SearchStrings(found, n) == len(found) {
			delete(d.Dirs, n)
		}
	}
}

func (t *treeBuilder) updateDirTree(dir *Directory, fset *token.FileSet) *Directory {
	fi, err := os.Stat(dir.Path)
	if err != nil || !fi.IsDir() {
		return nil
	}
	var dirchs []chan *Directory
	if sameFile(fi, dir.Info) {
		// Update Package
		if dir.Pkg != nil {
			// TODO: Handle Package errore
			dir.Pkg, _ = t.c.updatePackage(dir.Pkg, fi, fset, nil)
		}
		// Start updates before updating Package.
		for _, d := range dir.Dirs {
			ch := make(chan *Directory)
			dirchs = append(dirchs, ch)
			go func(d *Directory) {
				ch <- t.updateDirTree(d, fset)
			}(d)
		}
	} else {
		list, err := readdirnames(dir.Path)
		if err != nil {
			return nil
		}
		dir.Info = fi
		// Update or create Package
		switch {
		case dir.Pkg != nil:
			// TODO: Handle Package errors
			dir.Pkg, _ = t.c.updatePackage(dir.Pkg, fi, fset, list)

		case hasGoFiles(list):
			// Attempt to create a new package
			dir.Pkg, err = t.c.importPackage(dir.Path, fi, fset, list)
		}
		// Start update Goroutines
		for _, d := range list {
			if isPkgDir(d) {
				if _, ok := dir.Dirs[d]; !ok {
					ch := make(chan *Directory)
					dirchs = append(dirchs, ch)
					go func(d string) {
						ch <- t.newDirTree(fset, filepath.Join(dir.Path, d), d,
							dir.Depth+1, dir.Internal)
					}(d)
				}
			}
		}
		// Remove missing Dirs
		dir.removeNotFound(list)
		if len(dir.Dirs) == 0 && dir.Pkg == nil {
			return nil
		}
	}
	if dir.Pkg != nil {
		dir.PkgName = dir.Pkg.Name
		dir.HasPkg = true
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

func (t *treeBuilder) newDirTree(fset *token.FileSet, path, name string,
	depth int, internal bool) *Directory {

	if t.maxDepth != 0 && depth >= t.maxDepth {
		return &Directory{
			Depth: depth,
			Path:  path,
			Name:  name,
		}
	}
	// TODO: handle errors
	fi, err := os.Stat(path)
	if err != nil || !fi.IsDir() {
		return nil
	}
	list, err := readdirnames(path)
	if err != nil {
		return nil // Change
	}
	if !internal && isInternal(path) {
		internal = true
	}
	dir := &Directory{
		Path:     path,
		Name:     name,
		Internal: internal,
		Info:     fi,
		Depth:    depth,
		Dirs:     make(map[string]*Directory),
	}
	// TODO: handle errors
	pkg, _ := t.c.importPackage(path, fi, fset, list)
	if pkg != nil {
		dir.Pkg = pkg
		dir.PkgName = pkg.Name
		dir.HasPkg = pkg.isPkgDir()
	}
	var dirchs []chan *Directory
	for _, d := range list {
		if isPkgDir(d) {
			ch := make(chan *Directory)
			dirchs = append(dirchs, ch)
			go func(d string) {
				ch <- t.newDirTree(fset, filepath.Join(path, d), d, depth+1,
					internal)
			}(d)
		}
	}
	for _, ch := range dirchs {
		if d := <-ch; d != nil {
			dir.Dirs[d.Name] = d
		}
	}
	// if !dir.HasPkg && len(dir.Dirs) == 0 {
	if len(dir.Dirs) == 0 {
		// return nil
	}
	return dir
}

func isInternal(p string) bool {
	return filepath.Base(p) == "internal"
}

func splitPath(p string) []string {
	p = strings.TrimPrefix(p, string(os.PathSeparator))
	if p == "" {
		return nil
	}
	return strings.Split(p, string(os.PathSeparator))
}

func (dir *Directory) Lookup(path string) *Directory {
	d := splitPath(dir.Path) // dir.Path assumed to be clearn
	p := splitPath(filepath.Clean(path))
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

// TODO (CEV): Not used anywhere, remove?
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
	list := make([]string, 0, 512)
	dir.listPkgs(filepathDir(path), &list)
	sort.Strings(list)
	return list
}

// listPkgs, appends the absolute paths of Go packages importable from path to
// list, if path == "" internal import restrictions are ignored.
func (dir *Directory) listPkgs(path string, list *[]string) {
	if dir.Internal && !dir.matchInternal(path) {
		return
	}
	if dir.HasPkg {
		*list = append(*list, dir.Path)
	}
	for _, d := range dir.Dirs {
		d.listPkgs(path, list)
	}
}

// matchInternal, returns is path can import 'internal' directory d.
func (d *Directory) matchInternal(path string) bool {
	return d.Internal && path != "" && sameRoot(path, internalRoot(d.Path))
}

func listDirs(dir *Directory, list *[]string, path string) {
	*list = append(*list, dir.Path)
	if dir.Dirs != nil {
		for _, d := range dir.Dirs {
			listDirs(d, list, path)
		}
	}
}

// internalRoot, returns the parent directory of an internal package.
func internalRoot(path string) string {
	if n := strings.LastIndex(path, "internal"); n != -1 {
		root := filepath.Dir(path[:n])
		// Support Go 1.4
		if filepath.Base(root) == "pkg" {
			return filepath.Dir(root)
		}
		return root
	}
	return path
}

// sameRoot, returns if path is inside the directory tree rooted at root.
func sameRoot(path, root string) bool {
	return len(path) >= len(root) && path[0:len(root)] == root
}
