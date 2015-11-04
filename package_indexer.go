package pkg

import (
	"errors"
	"fmt"
	"go/token"
	"os"
	"path/filepath"
	"sync"
)

type PackageIndexer struct {
	c    *Corpus
	fset *token.FileSet
	mode ImportMode
	mu   sync.RWMutex

	// "$GOPATH/src" => "net/http" => Package
	packages map[string]map[string]*Package

	// stack of deleted packages, used to sync with Indexer
	deleted []Pak
}

func newPackageIndexer(c *Corpus) *PackageIndexer {
	return &PackageIndexer{
		c:        c,
		fset:     token.NewFileSet(),
		packages: make(map[string]map[string]*Package),
	}
}

func (x *PackageIndexer) lookupPath(path string) (p *Package) {
	if x.packages == nil {
		return nil
	}
	x.mu.RLock()
	defer x.mu.RUnlock()
	for root := range x.packages {
		if hasRoot(path, root) {
			path := trimPathPrefix(path, root)
			return x.packages[root][path]
		}
	}
	return nil
}

func (x *PackageIndexer) addPackage(p *Package) {
	x.mu.Lock()
	if x.packages == nil {
		x.packages = make(map[string]map[string]*Package)
	}
	if x.packages[p.Root] == nil {
		x.packages[p.Root] = make(map[string]*Package)
	}
	x.packages[p.Root][p.ImportPath] = p
	x.mu.Unlock()
}

func (x *PackageIndexer) deletePackage(p *Package) {
	if p == nil || x.packages == nil || x.packages[p.Root] == nil {
		return
	}
	x.mu.Lock()
	delete(x.packages[p.Root], p.ImportPath)
	if x.c.IndexGoCode {
		x.deleted = append(x.deleted, p.Pak())
	}
	x.mu.Unlock()
}

func (x *PackageIndexer) newPackage(dir string, fi os.FileInfo) (*Package, error) {
	if !fi.IsDir() {
		return nil, errors.New("pkg: invalid Package path")
	}
	p := &Package{
		Dir:            dir,
		mode:           x.mode,
		Info:           fi,
		GoFiles:        make(FileMap),
		IgnoredGoFiles: make(FileMap),
		TestGoFiles:    make(FileMap),
	}
	// Figure out if which Go path/root we're in.
	// SrcDirs returns $GOPATH + "/src" - so trim.
	for _, srcDir := range x.c.ctxt.SrcDirs() {
		if hasRoot(p.Dir, srcDir) {
			p.ImportPath = trimPathPrefix(p.Dir, srcDir)
			p.Root = filepath.Dir(srcDir)
			p.Goroot = hasRoot(p.Dir, x.c.ctxt.GOROOT())
			break
		}
	}
	return p, nil
}

// findPkgName, attempts to find and set the name of a Package with no
// buildable Go files.
func (x *PackageIndexer) findPkgName(p *Package) {
	if !p.isPkgDir() || p.Name != "" {
		return
	}
	for _, f := range p.IgnoredGoFiles {
		if n, ok := parseFileName(x.fset, f.Path); ok {
			p.Name = n
			return
		}
	}
	for _, f := range p.TestGoFiles {
		if n, ok := parseFileName(x.fset, f.Path); ok {
			p.Name = n
			return
		}
	}
}

// visitDirectory, updates or creates a Package for the Directory.  Used when
// building or updating the dir-tree.
func (x *PackageIndexer) visitDirectory(dir *Directory, names []string) *Package {
	if dir == nil {
		return nil
	}
	if p := x.lookupPath(dir.Path); p != nil {
		if err := x.updatePackage(p, dir.Info, names); err != nil {
			x.deletePackage(p)
			return nil
		}
		return p
	}
	if p, _ := x.importPackage(dir.Path, dir.Info, names); p != nil {
		x.addPackage(p)
		return p
	}
	return nil
}

// importDir, imports the Package located at directory dir.
func (x *PackageIndexer) importDir(dir string) (*Package, error) {
	fi, err := os.Stat(dir)
	if err != nil {
		return nil, err
	}
	if !fi.IsDir() {
		return nil, errors.New("pkg: invalid Package path")
	}
	names, err := readdirnames(dir)
	if err != nil {
		return nil, err
	}
	pkg, err := x.importPackage(dir, fi, names)
	if err != nil {
		return nil, err
	}
	x.addPackage(pkg)
	return pkg, nil
}

// updatePackage, updates Package p, FileInfo fi is required, the directory
// name slice is optional.  If an error is returned the Package should be
// removed from the index.
func (x *PackageIndexer) updatePackage(p *Package, fi os.FileInfo, names []string) error {
	if !fi.IsDir() {
		return errors.New("pkg: invalid Package path: " + p.Dir)
	}
	p.mode = x.mode
	if !x.c.IndexFileInfo && p.Name != "" {
		p.Info = fi
		return nil
	}
	names, err := x.readdirnames(p, names, sameFile(p.Info, fi))
	if err != nil {
		return err
	}
	p.Info = fi
	for _, name := range names {
		if err := x.updateFile(p, name); err != nil {
			if e, ok := err.(*MultiplePackageError); ok {
				p.err = e
				return nil
			}
		}
	}
	// Remove missing files.
	p.removeNotSeen(names)
	if !p.isPkgDir() {
		return &NoGoError{Dir: p.Dir}
	}
	// Attempt to find the package name.
	if p.Name == "" && p.err == nil {
		x.findPkgName(p)
		p.err = &NoBuildableGoError{Dir: p.Dir}
	}
	return nil
}

// readdirnames, returns the names from Package p's directory, and if used for
// updates.  The purpose of this function is to reduce contention for IO.
//
// If names are not nil they are returned.  Otherwise if usePkgFiles is true
// (no change to directory FileInfo) presumably no files have been added or
// deleted so we can just return a slice of the Go files already stored in the
// Package.  If names are nil and usePkgFiles is false we read the names from
// Package.Dir (os.Readdirnames).
func (x *PackageIndexer) readdirnames(p *Package, names []string,
	usePkgFiles bool) ([]string, error) {

	if names != nil {
		return names, nil
	}
	if usePkgFiles {
		return p.fileNames(), nil
	}
	return readdirnames(p.Dir)
}

func (x *PackageIndexer) updateFile(p *Package, name string) error {
	if !isGoFile(name) {
		return nil
	}
	f, ok := p.LookupFile(name)
	if !ok {
		return x.addFile(p, name)
	}
	// Check file info, if it's the same return early.
	if x.c.IndexFileInfo {
		fi, err := os.Stat(f.Path)
		if err != nil || fi.IsDir() {
			p.deleteFile(name)
			return err
		}
		same := sameFile(f.Info, fi)
		f.Info = fi
		if same {
			return nil
		}
	}
	return x.indexFile(p, f)
}

// importPackage, returns details about the Go package rooted at dir.
func (x *PackageIndexer) importPackage(dir string, fi os.FileInfo, names []string) (*Package, error) {
	if !hasGoFiles(names) {
		return nil, &NoGoError{Dir: dir}
	}
	p, err := x.newPackage(dir, fi)
	if err != nil {
		return nil, err
	}
	for _, name := range names {
		if err := x.addFile(p, name); err != nil {
			if e, ok := err.(*MultiplePackageError); ok {
				p.err = e
				return p, err
			}
		}
	}
	if !p.isPkgDir() {
		return nil, &NoGoError{Dir: dir}
	}
	if p.Name == "" {
		x.findPkgName(p)
		p.err = &NoBuildableGoError{Dir: dir}
	}
	return p, nil
}

func (x *PackageIndexer) addFile(p *Package, name string) error {
	if !isGoFile(name) {
		return nil
	}
	path := filepath.Join(p.Dir, name)
	f, err := NewFile(path, x.c.IndexFileInfo)
	if err != nil {
		return err
	}
	return x.indexFile(p, f)
}

func (x *PackageIndexer) indexFile(p *Package, f File) error {
	// Only parse files that match the current build.Context
	index := false
	switch {
	case isGoTestFile(f.Name):
		// We don't check the build tags of test files,
		// and since test files are determined by f.Name
		// we don't need to check the other file maps.
		p.TestGoFiles[f.Name] = f

	case x.c.ctxt.MatchFile(p.Dir, f.Name):
		delete(p.IgnoredGoFiles, f.Name)
		p.GoFiles[f.Name] = f
		index = true

	default:
		delete(p.GoFiles, f.Name)
		p.IgnoredGoFiles[f.Name] = f
	}
	switch {
	case !index:
		// Nothing to do...

	case p.FindPackageFiles():
		// Parse every file, checking for MultiplePackageError.
		name, ok := parseFileName(x.fset, f.Path)
		if !ok {
			return nil
		}
		switch p.Name {
		case name:
			// Ok
		case "":
			p.Name = name
		default:
			var firstFile string
			for _, f := range p.GoFiles {
				firstFile = f.Name
				break
			}
			return &MultiplePackageError{
				Dir:      p.Dir,
				Packages: []string{p.Name, name},
				Files:    []string{firstFile, f.Name},
			}
		}

	case p.FindPackageName():
		if p.Name == "" {
			if name, ok := parseFileName(x.fset, f.Path); ok {
				p.Name = name
			}
		}
	}
	return nil
}

// NoGoError is the error used by Import to describe a directory
// containing no Go source files.
type NoGoError struct {
	Dir string
}

func (e *NoGoError) Error() string {
	return "no buildable Go source files in " + e.Dir
}

// NoGoError is the error used by Import to describe a directory
// containing no buildable Go source files. (It may still contain
// test files, files hidden by build tags, and so on.)
type NoBuildableGoError struct {
	Dir string
}

func (e *NoBuildableGoError) Error() string {
	return "no buildable Go source files in " + e.Dir
}

// MultiplePackageError describes a directory containing
// multiple buildable Go source files for multiple packages.
type MultiplePackageError struct {
	Dir      string   // directory containing files
	Packages []string // package names found
	Files    []string // corresponding files: Files[i] declares package Packages[i]
}

func (e *MultiplePackageError) Error() string {
	// Error string limited to two entries for compatibility.
	return fmt.Sprintf("found packages %s (%s) and %s (%s) in %s", e.Packages[0],
		e.Files[0], e.Packages[1], e.Files[1], e.Dir)
}
