package pkg

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	pathpkg "path"
	"path/filepath"
	"sort"
	"sync"

	"github.com/charlievieth/pkg/fs"
	"github.com/charlievieth/pkg/util"
)

type File struct {
	Name string      // file name
	Path string      // absolute file path
	Info os.FileInfo // file info, used for updating
}

// TODO: Remove if unused.
func NewFile(path string, info bool) (File, error) {
	f := File{
		Name: filepath.Base(path),
		Path: path,
	}
	if info {
		fi, err := fs.Stat(path)
		if err != nil {
			return File{}, err
		}
		if fi.IsDir() {
			return File{}, errors.New("pkg: invalid path for file: " + path)
		}
		f.Info = fi
	}
	return f, nil
}

func (f *File) IsValid() bool {
	return f.Name != "" && f.Path != ""
}

func (f File) String() string {
	// Here to make debugging a little easier.
	const s = "{Name:%s Path:%s Info:{Name:%s Size:%d Mode:%s ModTime:%s IsDir:%v}}"
	return fmt.Sprintf(s, f.Name, f.Path, f.Info.Name(), f.Info.Size(),
		f.Info.Mode(), f.Info.ModTime(), f.Info.IsDir())
}

type byFileName []File

func (f byFileName) Len() int           { return len(f) }
func (f byFileName) Less(i, j int) bool { return f[i].Name < f[j].Name }
func (f byFileName) Swap(i, j int)      { f[i].Name, f[j].Name = f[j].Name, f[i].Name }

// A FileMap is a map of related files.
type FileMap map[string]File

// Files, returns the files in the FileMap as a slice.
func (m FileMap) Files() []File {
	s := m.appendFiles(make([]File, 0, len(m)))
	sort.Sort(byFileName(s))
	return s
}

// Files, returns the file names in the FileMap as a slice.
func (m FileMap) FileNames() []string {
	s := m.appendFileNames(make([]string, 0, len(m)))
	sort.Strings(s)
	return s
}

// Files, returns the file paths in the FileMap as a slice.
func (m FileMap) FilePaths() []string {
	s := m.appendFilePaths(make([]string, 0, len(m)))
	sort.Strings(s)
	return s
}

func (m FileMap) appendFiles(s []File) []File {
	for _, f := range m {
		s = append(s, f)
	}
	return s
}

func (m FileMap) appendFileNames(s []string) []string {
	for _, f := range m {
		s = append(s, f.Name)
	}
	return s
}

func (m FileMap) appendFilePaths(s []string) []string {
	for _, f := range m {
		s = append(s, f.Path)
	}
	return s
}

// removeNotSeen, removes files not present in sorted slice seen.
func (m FileMap) removeNotSeen(seen map[string]struct{}) {
	for name, file := range m {
		if _, ok := seen[file.Name]; !ok {
			delete(m, name)
		}
	}
}

// first, returns the first File from the map, note this is not guaranteed to
// be the first file added.
func (m FileMap) first() File {
	for _, f := range m {
		return f
	}
	return File{}
}

// A GoFileType describes a Go file in a package directory.
type GoFileType int

const (
	IgnoredGoFile GoFileType = 1 + iota // .go source files (excluding TestGoFiles and IgnoredGoFiles)
	TestGoFile                          // .go source files ignored for this build
	GoFile                              // _test.go files in package (build tags are not checked)
)

var goFileTypeStr = [...]string{
	"IgnoredGoFile",
	"TestGoFile",
	"GoFile",
}

func (t GoFileType) IsValid() bool {
	return IgnoredGoFile <= t && t <= GoFile
}

func (t GoFileType) String() string {
	if t.IsValid() {
		return goFileTypeStr[t]
	}
	return "Invalid"
}

// A Package describes a Go package or command.
type Package struct {
	Dir        string                 // Directory path "$GOROOT/src/net/http"
	Name       string                 // Package name "http"
	ImportPath string                 // Import path of package "net/http"
	Root       string                 // Root of Go tree where this package lives
	SrcRoot    string                 // package source root directory
	Goroot     bool                   // Package found in Go root
	Installed  bool                   // True if the package or command is installed
	Info       os.FileInfo            // File info as of last update
	files      map[GoFileType]FileMap // Go source files indexed by type
	err        error                  // Either NoGoError of MultiplePackageError
}

// Error, returns either NoGoError or MultiplePackageError.
func (p *Package) Error() error {
	return p.err
}

// IsCommand reports whether the package is considered a command to be installed
// (not just a library). Packages named "main" are treated as commands.
func (p *Package) IsCommand() bool {
	return p.Name == "main"
}

func (p *Package) IsValid() bool {
	return p.Name != "" && p.isPkgDir()
}

// GoFiles, returns a slice of buildable Go source files in the package.
func (p *Package) GoFiles() []string {
	return p.files[GoFile].FileNames()
}

func (p *Package) LookupFile(name string) (File, bool) {
	for _, m := range p.files {
		if m == nil {
			continue
		}
		if f, ok := m[name]; ok {
			return f, true
		}
	}
	return File{}, false
}

// fileLen, returns the number of files that match GoFileType typ.
func (p *Package) fileLen(typ GoFileType) int {
	n := 0
	for t, m := range p.files {
		if typ < 0 || t&typ != 0 {
			n += len(m)
		}
	}
	return n
}

// File, returns the files that match GoFileType typ.
// If GoFileType typ is less than zero all files are matched.
func (p *Package) Files(typ GoFileType) []File {
	s := make([]File, 0, p.fileLen(typ))
	for t, m := range p.files {
		if typ < 0 || t&typ != 0 {
			s = m.appendFiles(s)
		}
	}
	sort.Sort(byFileName(s))
	return s
}

// FileNames, returns the names of files that match GoFileType typ.
// If GoFileType typ is less than zero all files are matched.
func (p *Package) FileNames(typ GoFileType) []string {
	s := make([]string, 0, p.fileLen(typ))
	for t, m := range p.files {
		if typ < 0 || t&typ != 0 {
			s = m.appendFileNames(s)
		}
	}
	sort.Strings(s)
	return s
}

// FilePaths, returns the paths of files that match GoFileType typ.
// If GoFileType typ is less than zero all files are matched.
func (p *Package) FilePaths(typ GoFileType) []string {
	s := make([]string, 0, p.fileLen(typ))
	for t, m := range p.files {
		if typ < 0 || t&typ != 0 {
			s = m.appendFilePaths(s)
		}
	}
	sort.Strings(s)
	return s
}

func (p *Package) addFile(typ GoFileType, f File) {
	if p.files == nil {
		p.files = make(map[GoFileType]FileMap)
	}
	if p.files[typ] == nil {
		p.files[typ] = make(FileMap)
	}
	p.files[typ][f.Name] = f
	for t, m := range p.files {
		if t != typ && m != nil {
			delete(m, f.Name)
		}
	}
}

func (p *Package) removeFile(name string) {
	for _, m := range p.files {
		delete(m, name)
	}
}

// isPkgDir, returns if the Package contains any source files.
func (p *Package) isPkgDir() bool {
	for _, m := range p.files {
		if len(m) != 0 {
			return true
		}
	}
	return false
}

// removeNotSeen, removes any files not listed in seen.
func (p *Package) removeNotSeen(seen map[string]struct{}) {
	if !p.isPkgDir() {
		return
	}
	for _, m := range p.files {
		m.removeNotSeen(seen)
	}
}

type PackageIndex struct {
	c           *Corpus
	packages    map[string]map[string]*Package // "$GOROOT/src" => "net/http" => Package
	packagePath map[string]string              // "http" => "$GOROOT/src/net/http"
	strings     util.StringInterner
	mu          sync.RWMutex
}

func newPackageIndex(c *Corpus) *PackageIndex {
	return &PackageIndex{
		c:        c,
		packages: make(map[string]map[string]*Package),
	}
}

func (x *PackageIndex) notify(typ EventType, path string) {
	if x.c == nil {
		return
	}
	e := Event{
		typ: typ,
		msg: fmt.Sprintf("Package: %s %q", typ.color(), path),
	}
	x.c.notify(e)
}

func (p *PackageIndex) intern(s string) string {
	return p.strings.Intern(s)
}

func (x *PackageIndex) matchFile(p *Package, name string) bool {
	if x.c == nil || x.c.ctxt == nil {
		// Internal error
		panic("pkg: internal error (PackageIndex.matchFile)")
	}
	return x.c.ctxt.MatchFile(p.Dir, name)
}

// addPackage, adds package p to the index.
func (x *PackageIndex) addPackage(p *Package) {
	x.mu.Lock()
	if x.packages == nil {
		x.packages = make(map[string]map[string]*Package)
	}
	if x.packages[p.SrcRoot] == nil {
		x.packages[p.SrcRoot] = make(map[string]*Package)
	}
	x.packages[p.SrcRoot][p.ImportPath] = p

	if !p.IsCommand() {
		if x.packagePath == nil {
			x.packagePath = make(map[string]string)
		}
		x.packagePath[p.Name] = p.Dir
	}
	x.mu.Unlock()
}

// lookup returns the package located at path in directory root, if any.
func (x *PackageIndex) lookup(root, path string) (pkg *Package, ok bool) {
	x.mu.RLock()
	if x.packages != nil && x.packages[root] != nil {
		pkg, ok = x.packages[root][path]
	}
	x.mu.RUnlock()
	return
}

// lookupPath returns the package located at path, if any.
func (x *PackageIndex) lookupPath(path string) (*Package, bool) {
	if root := x.matchSrcRoot(path); root != "" {
		return x.lookup(root, trimPathPrefix(path, root))
	}
	return nil, false
}

// lookupPackage returns a package by name.  For example "http" should return
// the "net/http" package located at "$GOROOT/src/net/http".
func (x *PackageIndex) lookupPackage(name string) (*Package, bool) {
	if x.packages == nil || x.packagePath == nil {
		return nil, false
	}
	x.mu.Lock()
	if path, ok := x.packagePath[name]; ok {
		x.mu.Unlock()
		return x.lookupPath(path)
	}
	x.mu.Unlock()
	return nil, false
}

// remove, removes the package located at path from directory root.
func (x *PackageIndex) remove(root, path string) {
	if x.packages == nil || x.packagePath == nil {
		return
	}
	x.mu.Lock()
	if m := x.packages[root]; m != nil {
		if _, ok := m[path]; ok {
			delete(m, path)
			x.notify(DeleteEvent, path)
		}
	}
	name := pathpkg.Base(path)
	if x.packagePath[name] == pathpkg.Join(root, path) {
		delete(x.packagePath, name)
	}
	x.mu.Unlock()
}

// removePath removes the package rooted at path from the index.
func (x *PackageIndex) removePath(path string) {
	if root := x.matchSrcRoot(path); root != "" {
		x.remove(root, trimPathPrefix(path, root))
	}
}

// TODO: Remove if unused.
func (x *PackageIndex) ImportDir(dir string) (*Package, error) {
	fi, err := fs.Stat(dir)
	if err != nil || !fi.IsDir() {
		return nil, err
	}
	list, err := fs.Readdir(dir)
	if err != nil {
		return nil, err
	}
	return x.indexPkg(dir, fi, list)
}

// matchSrcRoot, returns the GOPATH/GOROOT that contains path.
func (x *PackageIndex) matchSrcRoot(path string) string {
	for _, srcDir := range x.c.ctxt.SrcDirs() {
		if hasRoot(path, srcDir) {
			return srcDir
		}
	}
	return ""
}

// isInstalled, returns if package is installed.
func (x *PackageIndex) isInstalled(p *Package) bool {
	if p.Root == "" {
		return false
	}
	var target string
	if p.IsCommand() {
		target = pathpkg.Join(p.Root, "bin", pathpkg.Base(p.ImportPath))
	} else {
		_, pkga, err := x.c.ctxt.PkgTargetRoot(p.ImportPath)
		if err != nil {
			return false
		}
		target = pathpkg.Join(p.Root, pkga)
	}
	return fs.IsFile(target)
}

func (x *PackageIndex) UpdatePackage(p *Package) (*Package, error) {
	if p == nil {
		return nil, errors.New("pkg: cannot update nil package")
	}
	fi, err := fs.Stat(p.Dir)
	if err != nil {
		x.remove(p.SrcRoot, p.ImportPath)
		return nil, err
	}
	return x.updatePkg(p.Dir, fi)
}

func (x *PackageIndex) InvalidateContext(matchFiles bool) {
	for _, m := range x.packages {
		for _, p := range m {
			x.updatePkgContext(p, matchFiles)
		}
	}
}

func (x *PackageIndex) updatePkgContext(p *Package, matchFiles bool) {
	if matchFiles {
		for _, f := range p.Files(GoFile | IgnoredGoFile) {
			if x.matchFile(p, f.Name) {
				p.addFile(GoFile, f)
			} else {
				p.addFile(IgnoredGoFile, f)
			}
		}
	}
	p.Installed = x.isInstalled(p)
}

func (x *PackageIndex) updatePkg(dir string, fi os.FileInfo) (*Package, error) {
	exitErr := func(err error) (*Package, error) {
		x.removePath(dir)
		return nil, err
	}
	if !isPkgDir(fi) {
		return exitErr(&NoGoError{dir})
	}
	p, pkgFound := x.lookupPath(dir)
	if p == nil || !pkgFound || !fs.SameFile(p.Info, fi) {
		// Stat only Go files.
		files, err := fs.ReaddirFunc(dir, fs.FilterGo)
		if err != nil {
			return exitErr(err)
		}
		return x.indexPkg(dir, fi, files)
	}

	// If the directory did not change, we can just stat
	// the previously indexed files and use that as the
	// file list to indexPkg.
	//
	// The goal here is to minimize the number of files
	// that we open as file system contention accounts
	// for the majority of the runtime.
	files := make([]os.FileInfo, 0, p.fileLen(-1))
	for _, m := range p.files {
		for _, f := range m {
			fi, err := fs.Stat(f.Path)
			if err != nil {
				p.removeFile(f.Name)
			} else {
				files = append(files, fi)
			}
		}
	}
	return x.indexPkg(dir, fi, files)
}

// indexPkg, indexes the package found at dir.
func (x *PackageIndex) indexPkg(dir string, fi os.FileInfo, files []os.FileInfo) (*Package, error) {
	// TODO: Write doc for this monster.
	// TODO: Test if we need to use filepath.EvalSymlinks to prevent duplicate
	// entries and other gremlins.

	srcRoot := x.matchSrcRoot(dir)
	if srcRoot == "" {
		return nil, fmt.Errorf("pkg: missing srcRoot for dir %q", dir)
	}
	importPath := trimPathPrefix(dir, srcRoot)

	if !isPkgDir(fi) || !hasGoFiles(files) {
		x.remove(dir, importPath)
		return nil, &NoGoError{dir}
	}

	p, pkgFound := x.lookup(srcRoot, importPath)
	if !pkgFound {
		// Create a new package.
		root := pathpkg.Dir(srcRoot)
		goroot := x.c.ctxt.GOROOT()
		p = &Package{
			Dir:        x.intern(dir),
			ImportPath: x.intern(importPath),
			Root:       x.intern(root),
			SrcRoot:    x.intern(srcRoot),
			Goroot:     hasRoot(dir, goroot),
			Info:       fi,
			files:      make(map[GoFileType]FileMap),
		}
	}

	// Removes the package from the index on error.
	exitErr := func(err error) (*Package, error) {
		if pkgFound {
			x.remove(srcRoot, importPath)
		}
		return nil, err
	}

	// Set error to nil, if whatever triggered
	// it is still present it will be reset.
	p.err = nil

	// If Go code indexing is enabled we will pass
	// the AST that we parsed here to the Index.
	updateAst := false
	astFiles := make(map[string]*ast.File)
	fset := token.NewFileSet()

	// TODO: Use the files slice
	//
	// Used for removing deleted/missing files.
	seen := make(map[string]struct{}, len(files))

	// Add new files and update any that changed.
	for _, fi := range files {
		seen[fi.Name()] = struct{}{}
		if !isGoFile(fi) {
			continue
		}

		name := fi.Name()
		f, found := p.LookupFile(name)
		if !found {
			// Create a new file.
			path := pathpkg.Join(p.Dir, name)
			f = File{
				Name: x.intern(name),
				Path: x.intern(path),
				Info: fi,
			}
		}
		same := fs.SameFile(f.Info, fi)
		f.Info = fi

		// Update AST if the file changed or is new.
		updateAst = updateAst || !same || !found

		switch {
		case same && found:
			// No changes, and the file is already indexed.

		case isGoTestFile(fi):
			// Don't parse Go test files.
			p.addFile(TestGoFile, f)

		case !x.matchFile(p, f.Name):
			// Ignored Go file.
			p.addFile(IgnoredGoFile, f)

		default:
			// Buildable Go file.
			//
			// If we are indexing Go code, parse the entire file.
			// This saves us from having to open/read/parse the
			// file twice.
			mode := parser.PackageClauseOnly
			if x.c.IndexGoCode {
				mode = parser.ParseComments
			}

			af, err := parseFile(fset, f.Path, mode)
			if err != nil {
				break
			}

			pkgName := af.Name.Name
			if !x.setPackageName(p, f.Name, pkgName) {
				p.Installed = false
				x.addPackage(p)
				return p, err
			}
			p.addFile(GoFile, f)
			astFiles[pkgName] = af
		}
	}

	// Remove deleted files from the package.
	p.removeNotSeen(seen)

	// No Go source files
	if !p.isPkgDir() {
		return exitErr(&NoGoError{dir})
	}

	// If there are no buildable Go source files the package
	// name will not have been set, attempt to set it via the
	// ignored Go source files.
	if p.Name == "" && len(p.files[IgnoredGoFile]) != 0 {
		// TODO: PkgNameLoop can probably be removed.
	PkgNameLoop:
		for _, f := range p.files[IgnoredGoFile] {
			if !x.parseFileName(fset, p, f) {
				if p.Error() != nil {
					break PkgNameLoop
				}
			}
		}
		// If there were parse errors we may have
		// removed all the Go source files.
		if !p.isPkgDir() {
			return exitErr(&NoGoError{})
		}
		// TODO: Parse test files, or use a better error.
		if p.Name == "" {
			return exitErr(&NoGoError{dir})
		}
	}

	p.Installed = x.isInstalled(p)
	x.addPackage(p)

	// Send notification.
	switch {
	case !pkgFound:
		x.notify(CreateEvent, p.Dir)
	case pkgFound && updateAst:
		x.notify(UpdateEvent, p.Dir)
	}

	// Index package idents
	if x.c.IndexGoCode && updateAst {
		// WARN: Make sure we parsed all pkg files!
		x.c.idents.indexPackageFiles(p, fset, astFiles)
	}
	return p, nil
}

// setPackageName, sets the package name and checks for multiple package errors.
func (x *PackageIndex) setPackageName(p *Package, fileName, pkgName string) bool {
	// TODO: Consider setting the error Package error.
	switch {
	case p.Name == "":
		p.Name = x.intern(pkgName)
	case p.Name != pkgName:
		first := p.files[GoFile].first().Name
		p.err = &MultiplePackageError{
			Dir:      p.Dir,
			Packages: []string{p.Name, pkgName},
			Files:    []string{first, fileName},
		}
	}
	return p.err == nil
}

// parseFileName, parses the package name of File f and sets the name of
// package p.  A MultiplePackageError is returned if the parsed name does
// not match the package name.
func (x *PackageIndex) parseFileName(fset *token.FileSet, p *Package, f File) bool {
	if name, ok := parseFileName(fset, f.Path); ok {
		return x.setPackageName(p, f.Name, name)
	}
	return false
}

// NoGoError is the error used by Import to describe a directory
// containing no Go source files.
type NoGoError struct {
	Dir string
}

func (e *NoGoError) Error() string {
	return "no buildable Go source files in " + e.Dir
}

// Returns, if the error err is NoGoError error.
func IsNoGo(err error) bool {
	_, ok := err.(*NoGoError)
	return ok
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

// Returns, if the error err is NoBuildableGoError error.
func IsNoBuildableGo(err error) bool {
	_, ok := err.(*NoBuildableGoError)
	return ok
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

// Returns, if the error err is MultiplePackageError error.
func IsMultiplePackage(err error) bool {
	_, ok := err.(*MultiplePackageError)
	return ok
}
