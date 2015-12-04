package pkg

import (
	"fmt"
	"os"
	pathpkg "path"
	"strings"
	"sync"

	"git.vieth.io/pkg/fs"
)

const defaultMaxDepth = 512

type treeBuilder struct {
	c        *Corpus
	x        *PackageIndex
	maxDepth int
	names    map[string]bool // dirs names - to prevent loops
	mu       sync.Mutex      // mutext for names map
}

func newTreeBuilder(c *Corpus, maxDepth int) *treeBuilder {
	if maxDepth <= 0 {
		maxDepth = 1e6
	}
	return &treeBuilder{
		c:        c,
		x:        c.packages,
		maxDepth: maxDepth,
		names:    make(map[string]bool),
	}
}

func (t *treeBuilder) notify(typ EventType, path string) {
	if t.c == nil || !t.c.LogEvents {
		return
	}
	e := Event{
		typ: typ,
		msg: fmt.Sprintf("DirTree: %s %q", typ.color(), path),
	}
	t.c.notify(e)
}

// seen, reports if the path has been seen.
func (t *treeBuilder) seen(path string) (ok bool) {
	t.mu.Lock()
	if ok = t.names[path]; !ok {
		t.names[path] = true
	}
	t.mu.Unlock()
	return ok
}

// updateDirTree, updates and returns Directory dir and all sub-directories.
// If the directory structure changed sub-directories are added and removed,
// accordingly.  Nil is returned if the path pointed to by dir is no longer
// a directory or an error was encountered.
func (t *treeBuilder) updateDirTree(dir *Directory) *Directory {
	// exitErr, deletes all Packages rooted at d.
	exitErr := func(d *Directory) *Directory {
		t.removePackage(d)
		return nil
	}
	if t.seen(dir.Path) || isIgnored(dir.Name) {
		return exitErr(dir)
	}

	// At or below MaxDepth, just return dir without checking
	// FileInfo or any sub-directories.
	if t.maxDepth != 0 && dir.Depth >= t.maxDepth {
		return dir
	}

	fi, err := fs.Stat(dir.Path)
	if err != nil || !fi.IsDir() {
		return exitErr(dir)
	}
	// noChange, means the directory structure should be the same.
	noChange := fs.SameFile(dir.Info, fi)
	dir.Info = fi

	// If there is no change to the directory, simply update any
	// existing sub-directories.
	//
	// Otherwise, read the directory dir and update, add and remove
	// sub-directories.
	var dirchs []chan *Directory
	if noChange {
		if dir.HasPkg {
			pkg, _ := t.x.updatePkg(dir.Path, dir.Info)
			if pkg != nil {
				dir.PkgName = pkg.Name
				dir.HasPkg = pkg.isPkgDir()
			}
		}
		for _, d := range dir.Dirs {
			ch := make(chan *Directory, 1)
			dirchs = append(dirchs, ch)
			go func(d *Directory) {
				ch <- t.updateDirTree(d)
			}(d)
		}
	} else {
		list, err := fs.Readdir(dir.Path)
		if err != nil {
			return exitErr(dir)
		}
		// Re-Index directory
		pkg, err := t.x.indexPkg(dir.Path, dir.Info, list)
		if err == nil {
			dir.PkgName = pkg.Name
			dir.HasPkg = pkg.isPkgDir()
		}
		for _, fi := range list {
			if isPkgDir(fi) {
				ch := make(chan *Directory, 1)
				dirchs = append(dirchs, ch)
				if d := dir.Dirs[fi.Name()]; d != nil {
					// Update existing sub-directory
					go func(d *Directory) {
						ch <- t.updateDirTree(d)
					}(d)
				} else {
					// Add new sub-directory
					go func(fi os.FileInfo) {
						path := pathpkg.Join(dir.Path, fi.Name())
						ch <- t.newDirTree(path, fi, dir.Depth, dir.Internal)
					}(fi)
				}
			}
		}
	}
	// Create sub-directory tree
	dirs := make(map[string]*Directory)
	for _, ch := range dirchs {
		if d := <-ch; d != nil {
			dirs[d.Name] = d
		}
	}
	if !dir.HasPkg && len(dirs) == 0 {
		return exitErr(dir)
	}
	// Check for removed sub-directories.
	for name, d := range dir.Dirs {
		if _, ok := dirs[name]; !ok {
			t.removePackage(d)
		}
	}
	// Do not assign until we know there are no errors.
	// Removing sub-directory packages requires the old
	// dirs map.
	dir.Dirs = dirs
	return dir
}

func (t *treeBuilder) newDirTree(path string, info os.FileInfo, depth int,
	internal bool) *Directory {

	name := info.Name()
	if t.seen(path) || isIgnored(name) {
		return nil
	}
	if t.maxDepth != 0 && depth >= t.maxDepth {
		// Return a dummy directory so that the
		// parent directory does not discard it.
		return &Directory{
			Depth:    depth,
			Path:     path,
			Name:     name,
			Internal: internal,
		}
	}
	list, err := fs.Readdir(path)
	if err != nil {
		return nil
	}

	// If the current name is "internal" set internal to true
	// so that all sub-directories will also be marked "internal".
	//
	// TODO (CEV): If supported, handle nested "internal" directories.
	if !internal && isInternal(path) {
		internal = true
	}

	// Index package.  To reduce strain on the filesystem
	// index before starting the sub-directory goroutines.
	var (
		pkgName string
		hasPkg  bool
	)
	if pkg, err := t.x.indexPkg(path, info, list); err == nil {
		pkgName = pkg.Name
		hasPkg = pkg.isPkgDir()
	}

	// Start goroutings to visit sub-directories
	var dirchs []chan *Directory
	for _, fi := range list {
		if isPkgDir(fi) {
			ch := make(chan *Directory, 1)
			dirchs = append(dirchs, ch)
			go func(fi os.FileInfo) {
				path := pathpkg.Join(path, fi.Name())
				ch <- t.newDirTree(path, fi, depth+1, internal)
			}(fi)
		}
	}

	// Create sub-directory tree
	dirs := make(map[string]*Directory)
	for _, ch := range dirchs {
		if d := <-ch; d != nil {
			dirs[d.Name] = d
		}
	}

	// If there is no package and no sub-directories containing
	// package files, ignore the directory.
	if !hasPkg && len(dirs) == 0 {
		return nil
	}

	t.notify(CreateEvent, path)
	return &Directory{
		Path:     path,
		Name:     name,
		PkgName:  pkgName,
		HasPkg:   hasPkg,
		Internal: internal,
		Info:     info,
		Depth:    depth,
		Dirs:     dirs,
	}
}

// removePackage, removes any Packages rooted at dir.
func (t *treeBuilder) removePackage(dir *Directory) {
	if dir == nil {
		return
	}
	t.notify(DeleteEvent, dir.Path)
	if dir.HasPkg {
		t.x.removePath(dir.Path)
	}
	for d := range dir.iter(true) {
		t.removePackage(d)
	}
}

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
	p = strings.TrimPrefix(p, "/")
	if p == "" {
		return nil
	}
	return strings.Split(p, "/")
}

func (dir *Directory) lookup(path string) *Directory {
	d := splitPath(dir.Path)
	p := splitPath(clean(path))
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

// TODO: Include Golang license, this comes almost directly from godoc.

type DirEntry struct {
	Depth    int    // >= 0
	Height   int    // = DirList.MaxHeight - Depth, > 0
	Path     string // directory path; includes Name, relative to DirList root
	Name     string // directory name
	PkgName  string // package name, or "" if none
	HasPkg   bool   // true if the directory contains at least one package
	Internal bool   // true if the package is an "internal" package
}

type DirList struct {
	MaxHeight int // directory tree height, > 0
	List      []DirEntry
}

func (root *Directory) listing(skipRoot bool, filter func(string) bool) *DirList {
	if root == nil {
		return nil
	}

	// determine number of entries n and maximum height
	n := 0
	minDepth := 1 << 30 // infinity
	maxDepth := 0
	for d := range root.iter(skipRoot) {
		n++
		if minDepth > d.Depth {
			minDepth = d.Depth
		}
		if maxDepth < d.Depth {
			maxDepth = d.Depth
		}
	}
	maxHeight := maxDepth - minDepth + 1

	if n == 0 {
		return nil
	}

	// create list
	list := make([]DirEntry, 0, n)
	for d := range root.iter(skipRoot) {
		if filter != nil && !filter(d.Path) {
			continue
		}
		depth := d.Depth - minDepth
		e := DirEntry{
			Depth:    depth,
			Height:   maxHeight - depth,
			Path:     trimPathPrefix(d.Path, root.Path),
			Name:     d.Name,
			PkgName:  d.PkgName,
			HasPkg:   d.HasPkg,
			Internal: d.Internal,
		}
		list = append(list, e)
	}

	return &DirList{maxHeight, list}
}
