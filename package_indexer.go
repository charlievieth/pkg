package pkg

import (
	"errors"
	"fmt"
	"go/token"
	"os"
	"path/filepath"
	"sync"
)

type ImportMode int

const (
	// If FindPackageName is set,
	FindPackageName ImportMode = 1 << iota

	// If IndexPackage is set, Package files are indexed
	FindPackageFiles
)

// CEV: This is pretty ugly but unlike a map allows ImportModes to be marshaled
// in a set order.
var importModeStr = []struct {
	m ImportMode
	s string
}{
	{FindPackageName, "+FindPackageName"},
	{FindPackageFiles, "+FindPackageFiles"},
}

func (i ImportMode) String() string {
	var b []byte
	for _, m := range importModeStr {
		if i&m.m != 0 {
			b = append(b, m.s...)
		}
	}
	if len(b) != 0 {
		return string(b[1:])
	}
	return "Invalid"
}

// TODO (CEV): Fix confusing and misleading Package method names.

type PackageIndexer struct {
	c    *Corpus
	fset *token.FileSet
	mode ImportMode
	mu   sync.RWMutex

	// "$GOROOT/src" => "net/http" => Package
	packages map[string]map[string]*Package

	// stack of deleted packages, used to sync with Indexer
	deleted []Pak

	// stack of added packages, used to sync with Indexer
	added []Pak
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
	if x.packages[p.SrcRoot] == nil {
		x.packages[p.SrcRoot] = make(map[string]*Package)
	}
	x.packages[p.SrcRoot][p.ImportPath] = p
	x.mu.Unlock()
}

func (x *PackageIndexer) deletePackage(p *Package) {
	if p == nil || x.packages == nil || x.packages[p.Root] == nil {
		return
	}
	x.mu.Lock()
	delete(x.packages[p.Root], p.ImportPath)
	if x.c.IndexEnabled {
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
			p.SrcRoot = srcDir
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
	if dir == nil || (names != nil && !hasGoFiles(names)) {
		return nil
	}
	if x.visitDir(dir.Path, names) == nil {
		return x.lookupPath(dir.Path)
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
		// if err := x.addFile(p, name); err != nil {
		if err := x.visitFile(p, name); err != nil {
			if IsMultiplePackage(err) {
				p.err = err
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

func (x *PackageIndexer) visitDir(dir string, names []string) (err error) {
	fi, err := os.Stat(dir)
	if err != nil {
		return err
	}
	var p *Package
	if p = x.lookupPath(dir); p != nil {
		if !fi.IsDir() {
			x.deletePackage(p)
			return errors.New("pkg: invalid Package path")
		}
		// Swap info for sameFile comparison.
		p.Info, fi = fi, p.Info
		p.mode = x.mode
	} else {
		// Make sure there are Go files before
		// creating the package.
		if names != nil && !hasGoFiles(names) {
			x.deletePackage(p)
			return &NoGoError{Dir: dir}
		}
		// An error is returned if fi is not a directory.
		p, err = x.newPackage(dir, fi)
		if err != nil {
			return err
		}
	}
	names, err = x.readdirnames(p, names, sameFile(p.Info, fi))
	if err != nil {
		return err
	}
	// If there error is still present it will be re-set.
	p.err = nil
	for _, name := range names {
		if err := x.visitFile(p, name); err != nil {
			if IsMultiplePackage(err) {
				p.err = err
				break
			}
		}
	}
	p.removeNotSeen(names)
	if !p.isPkgDir() {
		x.deletePackage(p)
		return &NoGoError{Dir: p.Dir}
	}
	// Attempt to find the package name.
	if p.Name == "" && p.err == nil {
		x.findPkgName(p)
		p.err = &NoBuildableGoError{Dir: p.Dir}
	}
	x.addPackage(p)
	return nil
}

// updatePackage, updates Package p.  FileInfo fi and the directory name slice
// are optional.  If an error is returned the Package should be removed from
// the index.
func (x *PackageIndexer) updatePackage(p *Package, fi os.FileInfo, names []string) error {
	// TODO: Rename - this is really only used
	// when updating the dir tree.

	if p == nil {
		return errors.New("pkg: cannot update nil Package:")
	}
	if fi == nil {
		var err error
		fi, err = os.Stat(p.Dir)
		if err != nil {
			return err
		}
	}
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
		if err := x.visitFile(p, name); err != nil {
			if IsMultiplePackage(err) {
				p.err = err
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
	if usePkgFiles && p.isPkgDir() {
		return p.fileNames(), nil
	}
	return readdirnames(p.Dir)
}

func (x *PackageIndexer) visitFile(p *Package, name string) (err error) {
	if !isGoFile(name) {
		return nil
	}
	file, found := p.LookupFile(name)
	switch {
	case !found:
		// Create new file
		path := filepath.Join(p.Dir, name)
		file, err = NewFile(path, x.c.IndexFileInfo)
		if err != nil {
			return err
		}
	case found && x.c.IndexFileInfo:
		// Update file info
		fi, err := os.Stat(file.Path)
		if err != nil {
			p.deleteFile(name)
			return err
		}
		if fi.IsDir() {
			p.deleteFile(name)
			return errors.New("pkg: invalid file path")
		}
		file.Info, fi = fi, file.Info
		if sameFile(file.Info, fi) {
			return nil
		}
	}
	return x.indexFile(p, file)
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
		// Nada
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

func IsNoGo(err error) bool {
	_, ok := err.(*NoGoError)
	return ok
}

func IsNoBuildableGo(err error) bool {
	_, ok := err.(*NoBuildableGoError)
	return ok
}

func IsMultiplePackage(err error) bool {
	_, ok := err.(*MultiplePackageError)
	return ok
}

/*

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

*/

/* Currently on ice for using too much magic.

// visitPackage, Combines update and create into one method
func (x *PackageIndexer) visitPackage(dirpath string, fi os.FileInfo,
	names []string) (p *Package, err error) {

	// TODO: Rename - this is really only used
	// when updating the dir tree.

	defer func() {
		if err != nil && p != nil {
			x.deletePackage(p)
			p = nil
		} else {
			x.addPackage(p)
		}
	}()

	if fi == nil {
		if fi, err = os.Stat(dirpath); err != nil {
			return
		}
	}
	if !fi.IsDir() {
		err = errors.New("pkg: invalid Package path: " + dirpath)
		return
	}
	p = x.lookupPath(dirpath)
	if p == nil {
		if p, err = x.newPackage(dirpath, fi); err != nil {
			return
		}
	}
	p.mode = x.mode
	if !x.c.IndexFileInfo && p.Name != "" {
		p.Info = fi
		return
	}
	names, err = x.readdirnames(p, names, sameFile(p.Info, fi))
	if err != nil {
		return
	}
	p.Info = fi
	for _, name := range names {
		if err := x.updateFile(p, name); err != nil {
			if e, ok := err.(*MultiplePackageError); ok {
				p.err = e
				return p, nil
			}
		}
	}
	// Remove missing files.
	p.removeNotSeen(names)
	if !p.isPkgDir() {
		err = &NoGoError{Dir: p.Dir}
		return
	}
	// Attempt to find the package name.
	if p.Name == "" && p.err == nil {
		x.findPkgName(p)
		p.err = &NoBuildableGoError{Dir: p.Dir}
	}
	return
}
*/
