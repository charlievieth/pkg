package pkg

// TODO:
// 	- Consider removing Corupus.MaxDepth
//  - Consider making pkg selection version aware

import (
	"errors"
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

const defaultMaxDepth = 512

func newTreeBuilder(c *Corpus) *treeBuilder {
	depth := c.MaxDepth
	if depth <= 0 {
		depth = 1e6
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

func (c *Corpus) newDirectory(root string, maxDepth int) (*Directory, error) {
	fi, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !fi.IsDir() {
		return nil, errors.New("pkg: invalid directory root: " + root)
	}
	fset := token.NewFileSet()
	t := &treeBuilder{c: c, maxDepth: maxDepth}
	dir := t.newDirTree(fset, root, filepath.Base(root), 0, false)
	return dir, nil
}

func (c *Corpus) updateDirectory(dir *Directory, maxDepth int) (*Directory, error) {
	if dir == nil {
		return nil, errors.New("pkg: cannot update nil Directory")
	}
	fi, err := os.Stat(dir.Path)
	if err != nil {
		return nil, err
	}
	if !fi.IsDir() {
		return nil, errors.New("pkg: invalid directory root: " + dir.Path)
	}
	fset := token.NewFileSet()
	t := &treeBuilder{c: c, maxDepth: maxDepth}
	return t.updateDirTree(dir, fset), nil
}

func (d *Directory) removeNotSeen(seen []string) {
	m := make(map[string]bool, len(seen))
	for _, s := range seen {
		m[s] = true
	}
	for n := range d.Dirs {
		if !m[n] {
			delete(d.Dirs, n)
		}
	}
}

func (t *treeBuilder) updateDirTree(dir *Directory, fset *token.FileSet) *Directory {
	fi, err := os.Stat(dir.Path)
	if err != nil || !fi.IsDir() {
		return nil
	}
	// No change to the directory according to the file system.
	// TODO (CEV): Test granularity of Linux and Windows.
	noChange := sameFile(fi, dir.Info)
	dir.Info = fi
	var dirchs []chan *Directory
	if noChange {
		// Update Package
		if dir.Pkg != nil {
			// TODO: Handle Package errors
			dir.Pkg, _ = t.c.updatePackage(dir.Pkg, fi, fset, nil)
		}
		// To reduce IO contention update package before
		// updating sub-directories.
		for _, d := range dir.Dirs {
			ch := make(chan *Directory)
			dirchs = append(dirchs, ch)
			go func(d *Directory) {
				ch <- t.updateDirTree(d, fset)
			}(d)
		}
	} else {
		dir.Info = fi
		list, err := readdirnames(dir.Path)
		if err != nil {
			return nil
		}
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
		dir.removeNotSeen(list)
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
		return nil
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
	// To reduce IO contention update package before
	// updating sub-directories.
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
	d := splitPath(dir.Path) // dir.Path assumed to be clean
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

func (dir *Directory) LookupPackage(path string) *Package {
	if d := dir.Lookup(path); d != nil {
		return d.Pkg
	}
	return nil
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
	dir.listPkgPaths(filepathDir(path), &list)
	sort.Strings(list)
	return list
}

// listPkgPaths, appends the absolute paths of Go packages importable from path to
// list, if path == "" internal import restrictions are ignored.
func (dir *Directory) listPkgPaths(path string, list *[]string) {
	if dir.Internal && !dir.matchInternal(path) {
		return
	}
	if dir.HasPkg {
		*list = append(*list, dir.Path)
	}
	for _, d := range dir.Dirs {
		d.listPkgPaths(path, list)
	}
}

func (dir *Directory) listPackages(list *[]*Package) {
	if dir.Pkg != nil {
		*list = append(*list, dir.Pkg)
	}
	for _, d := range dir.Dirs {
		d.listPackages(list)
	}
}

// matchInternal, returns is path can import 'internal' directory d.
func (d *Directory) matchInternal(path string) bool {
	return d.Internal && path != "" && hasRoot(path, internalRoot(d.Path))
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
