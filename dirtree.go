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
	}
	exitErr := func() *Directory {
		if dir.Pkg != nil {
			t.c.pkgIndex.deletePackage(dir.Pkg)
		}
		return nil
	}
	// To reduce IO contention update package before
	// updating sub-directories.
	pkg := t.c.pkgIndex.visitDirectory(dir, list)
	if pkg != nil {
		dir.Pkg = pkg
		dir.PkgName = pkg.Name
		dir.HasPkg = true
	}
	var dirchs []chan *Directory
	for _, name := range list {
		if isPkgDir(name) {
			dirchs = append(dirchs, t.newSubDirTree(dir, fset, name))
		}
	}
	dirs := make(map[string]*Directory)
	for _, ch := range dirchs {
		if d := <-ch; d != nil {
			dirs[d.Name] = d
		}
	}
	if len(dirs) == 0 && !dir.HasPkg {
		return exitErr()
	}
	dir.Dirs = dirs
	return dir
}

func (t *treeBuilder) newSubDirTree(dir *Directory, fset *token.FileSet, name string) chan *Directory {
	ch := make(chan *Directory, 1)
	go func() {
		path := filepath.Join(dir.Path, name)
		ch <- t.newDirTree(fset, path, name, dir.Depth+1, dir.Internal)
	}()
	return ch
}

func (t *treeBuilder) updateDirTree(dir *Directory, fset *token.FileSet) *Directory {

	if t.maxDepth != 0 && dir.Depth >= t.maxDepth {
		return dir
	}

	// Remove Package, if any, on error.
	exitErr := func() *Directory {
		if dir.Pkg != nil {
			t.c.pkgIndex.deletePackage(dir.Pkg)
		}
		return nil
	}

	fi, err := os.Stat(dir.Path)
	if err != nil || !fi.IsDir() {
		return exitErr()
	}
	// No change to the directory according to the file system.
	// TODO (CEV): Test granularity of Linux and Windows.
	noChange := sameFile(fi, dir.Info)
	dir.Info = fi

	var dirchs []chan *Directory
	if noChange {
		if dir.Pkg != nil {
			dir.Pkg = t.c.pkgIndex.visitDirectory(dir, nil)
		}
		// To reduce IO contention update sub-directories
		// after updating the package.
		for _, d := range dir.Dirs {
			dirchs = append(dirchs, t.updateSubDirTree(d, fset))
		}
	} else {
		// Directory changed - check all files.
		list, err := readdirnames(dir.Path)
		if err != nil {
			return exitErr()
		}
		// Update or create Package
		if dir.Pkg != nil || hasGoFiles(list) {
			dir.Pkg = t.c.pkgIndex.visitDirectory(dir, list)
		}
		// Start update Goroutines
		for _, name := range list {
			if isPkgDir(name) {
				if d := dir.Dirs[name]; d != nil {
					dirchs = append(dirchs, t.updateSubDirTree(d, fset))
				} else {
					dirchs = append(dirchs, t.newSubDirTree(dir, fset, name))
				}
			}
		}
	}
	if dir.Pkg != nil {
		dir.PkgName = dir.Pkg.Name
		dir.HasPkg = true
	}
	dirs := make(map[string]*Directory)
	for _, ch := range dirchs {
		if d := <-ch; d != nil {
			dirs[d.Name] = d
		}
	}
	if len(dirs) == 0 && !dir.HasPkg {
		return exitErr()
	}
	dir.Dirs = dirs
	return dir
}

func (t *treeBuilder) updateSubDirTree(dir *Directory, fset *token.FileSet) chan *Directory {
	ch := make(chan *Directory, 1)
	go func() {
		ch <- t.updateDirTree(dir, fset)
	}()
	return ch
}

func (dir *Directory) walk(c chan<- *Directory, skipRoot bool) {
	if dir != nil {
		if !skipRoot {
			c <- dir
		}
		for _, d := range dir.Dirs {
			d.walk(c, false)
		}
	}
}

func (dir *Directory) iter(skipRoot bool) <-chan *Directory {
	c := make(chan *Directory)
	go func() {
		dir.walk(c, skipRoot)
		close(c)
	}()
	return c
}

func (dir *Directory) lookupLocal(name string) *Directory {
	if d, ok := dir.Dirs[name]; ok {
		return d
	}
	return nil
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

// matchInternal, returns is path can import 'internal' directory d.
func (d *Directory) matchInternal(path string) bool {
	return d.Internal && path != "" && hasRoot(path, internalRoot(d.Path))
}

func (dir *Directory) listPackages(list *[]*Package) {
	if dir.Pkg != nil {
		*list = append(*list, dir.Pkg)
	}
	for _, d := range dir.Dirs {
		d.listPackages(list)
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

func isInternal(p string) bool {
	return filepath.Base(p) == "internal"
}
