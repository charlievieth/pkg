package pkg

import (
	"errors"
	"fmt"
	"go/token"
	"os"
	"path/filepath"
	"sort"
)

type ImportMode int

const (
	// If FindPackageOnly is set, NewPackage stops after reading only package
	// statement.
	FindPackageOnly ImportMode = 1 << iota

	// If FindPackageName is set,
	FindPackageName

	// If IndexPackage is set, Package files are indexed
	FindPackageFiles
)

// CEV: This is pretty ugly but unlike a map allows ImportModes to be marshaled
// in a set order.
var importModeStr = []struct {
	m ImportMode
	s string
}{
	{FindPackageOnly, "+FindPackageOnly"},
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

var NoFile File

type File struct {
	Name string      // file name
	Path string      // absolute file path
	Info os.FileInfo // file info, used for updating
}

func NewFile(path string, info bool) (File, error) {
	f := File{
		Name: filepath.Base(path),
		Path: path,
	}
	if info {
		fi, err := os.Stat(path)
		if err != nil {
			return File{}, err
		}
		if fi.IsDir() {
			return File{}, errors.New("pkg: directory path: " + path)
		}
	}
	return f, nil
}

func (f *File) Valid() bool {
	return f.Name != "" && f.Path != ""
}

type ByFileName []File

func (f ByFileName) Len() int           { return len(f) }
func (f ByFileName) Less(i, j int) bool { return f[i].Name < f[j].Name }
func (f ByFileName) Swap(i, j int)      { f[i].Name, f[j].Name = f[j].Name, f[i].Name }

type FileMap map[string]File

func (m FileMap) Files() []File {
	fs := make([]File, 0, len(m))
	for _, f := range m {
		fs = append(fs, f)
	}
	sort.Sort(ByFileName(fs))
	return fs
}

type Package struct {
	Dir        string // directory path
	Name       string // package name
	ImportPath string // import path of package ("" if unknown)
	Root       string // root of Go tree where this package lives
	Goroot     bool   // package found in Go root

	// GoFiles        map[string]*File // .go source files (excluding TestGoFiles, XTestGoFiles)
	// IgnoredGoFiles map[string]*File // .go source files ignored for this build
	// TestGoFiles    map[string]*File // _test.go files in package

	GoFiles        FileMap // .go source files (excluding TestGoFiles, XTestGoFiles)
	IgnoredGoFiles FileMap // .go source files ignored for this build
	TestGoFiles    FileMap // _test.go files in package

	Info os.FileInfo

	mode ImportMode // ImportMode used when created
	err  error      // Either NoGoError of MultiplePackageError
}

// Error, returns
// Either NoGoError of MultiplePackageError
func (p *Package) Error() error {
	return p.err
}

func (p *Package) FindPackageOnly() bool {
	return p.mode&FindPackageOnly != 0
}

func (p *Package) FindPackageName() bool {
	return p.mode&FindPackageName != 0
}

func (p *Package) FindPackageFiles() bool {
	return p.mode&FindPackageFiles != 0
}

func (p *Package) LookupFile(name string) (File, bool) {
	if p.GoFiles != nil {
		if f, ok := p.GoFiles[name]; ok {
			return f, ok
		}
	}
	if p.IgnoredGoFiles != nil {
		if f, ok := p.IgnoredGoFiles[name]; ok {
			return f, ok
		}
	}
	if p.TestGoFiles != nil {
		if f, ok := p.TestGoFiles[name]; ok {
			return f, ok
		}
	}
	return File{}, false
}

func (p *Package) initMaps() {
	if p.GoFiles == nil {
		p.GoFiles = make(FileMap)
	}
	if p.IgnoredGoFiles == nil {
		p.IgnoredGoFiles = make(FileMap)
	}
	if p.TestGoFiles == nil {
		p.TestGoFiles = make(FileMap)
	}
}

func (p *Package) DeleteFile(name string) {
	delete(p.GoFiles, name)
	delete(p.IgnoredGoFiles, name)
	delete(p.TestGoFiles, name)
}

func (p *Package) isPkgDir() bool {
	return len(p.GoFiles) != 0 ||
		len(p.TestGoFiles) != 0 ||
		len(p.IgnoredGoFiles) != 0
}

// IsCommand reports whether the package is considered a command to be installed
// (not just a library). Packages named "main" are treated as commands.
func (p *Package) IsCommand() bool {
	return p.Name == "main"
}

// findPkgName, attempts to find the pkg name.  If there are no buildable
// Gofiles we don't parse any package names, this parses ignored and test
// files until a name is found.
func (p *Package) findPkgName(fset *token.FileSet) {
	if !p.isPkgDir() {
		return
	}
	for _, f := range p.IgnoredGoFiles {
		if n, ok := parseFileName(f.Path, fset); ok {
			p.Name = n
			return
		}
	}
	for _, f := range p.TestGoFiles {
		if n, ok := parseFileName(f.Path, fset); ok {
			p.Name = n
			return
		}
	}
}

// trimFiles, removes any files not listed in seen.
func (p *Package) trimFiles(seen []string) {
	m := make(map[string]bool, len(seen))
	for name := range p.GoFiles {
		if !m[name] {
			delete(p.GoFiles, name)
		}
	}
	for name := range p.TestGoFiles {
		if !m[name] {
			delete(p.TestGoFiles, name)
		}
	}
	for name := range p.IgnoredGoFiles {
		if !m[name] {
			delete(p.IgnoredGoFiles, name)
		}
	}
}

func (c *Corpus) NewPackage(dir string, mode ImportMode) *Package {
	p, _ := c.importPackage(dir, nil, token.NewFileSet(), nil)
	return p
}

// TODO: Organize args
func (c *Corpus) importPackage(dir string, fi os.FileInfo, fset *token.FileSet,
	names []string) (*Package, error) {

	p := &Package{
		Dir:            dir,
		mode:           c.PackageMode,
		Info:           fi,
		GoFiles:        make(FileMap),
		IgnoredGoFiles: make(FileMap),
		TestGoFiles:    make(FileMap),
	}
	// Figure out if which Go path/root we're in.
	// SrcDirs returns $GOPATH + "/src" - so trim.
	for _, srcDir := range c.ctxt.SrcDirs() {
		if sameRoot(dir, srcDir) {
			p.ImportPath = trimPathPrefix(dir, srcDir)
			p.Root = filepath.Dir(srcDir)
			p.Goroot = sameRoot(dir, c.ctxt.GOROOT())
			break
		}
	}
	// Found the Package, nothing else to do.
	if p.FindPackageOnly() {
		return p, nil
	}
	var first error
	for _, name := range names {
		if err := c.addFile(p, name, fset); err != nil {
			if e, ok := err.(*MultiplePackageError); ok {
				p.err = e
			}
			if first != nil {
				first = err
			}
		}
	}
	if !p.isPkgDir() {
		return nil, first
	}
	if p.Name == "" {
		// Attempt to find the package name.
		p.findPkgName(fset)
		first = &NoGoError{Dir: dir}
		p.err = first
	}
	return p, first
}

var ErrPackageNotExist = errors.New("pkg: package directory does not exists")

// TODO: Organize args
func (c *Corpus) updatePackage(p *Package, fi os.FileInfo, fset *token.FileSet,
	names []string) (*Package, error) {

	if !fi.IsDir() {
		return nil, ErrPackageNotExist
	}
	p.mode = c.PackageMode
	// Unless we are indexing fileinfo return, reading
	// in the dirnames on each update is very slow.
	if len(names) == 0 && !c.IndexFileInfo {
		return p, nil
	}
	names, err := completeDirnames(p.Dir, names)
	if err != nil {
		return nil, err
	}
	var (
		pkgErr error
		first  bool
	)
	// Set pkg err to nil, if it's still relevant
	// the update will re-set it.
	p.err = nil
	for _, name := range names {
		if !isGoFile(name) {
			continue
		}
		if first {
			// If the ImportMode changed, the maps may be nil.
			// This probably can't happen, but let's not panic.
			p.initMaps()
			first = false
		}
		if err := c.updateFile(p, name, fset); err != nil {
			if e, ok := err.(*MultiplePackageError); ok {
				p.err = e
			}
			if pkgErr != nil {
				pkgErr = err
			}
		}
	}
	// Remove missing files.
	p.trimFiles(names)
	if !p.isPkgDir() {
		return nil, pkgErr
	}
	if p.Name == "" {
		// Attempt to find the package name.
		p.findPkgName(fset)
		pkgErr = &NoGoError{Dir: p.Dir}
		p.err = pkgErr
	}
	return p, pkgErr
}

func (c *Corpus) updateFile(p *Package, name string, fset *token.FileSet) error {
	f, ok := p.LookupFile(name)
	if !ok {
		return c.addFile(p, name, fset)
	}
	if c.IndexFileInfo {
		fi, err := os.Stat(f.Path)
		if err != nil {
			p.DeleteFile(name)
			return err
		}
		if sameFile(f.Info, fi) {
			return nil
		}
		if fi.IsDir() {
			p.DeleteFile(name)
			return nil
		}
		f.Info = fi
	}
	index := false
	switch {
	case isGoTestFile(name):
		// We don't check the build tags of test files,
		// and since test files are determined by name
		// we don't need to check the other file maps.
		p.TestGoFiles[name] = f

	case c.ctxt.MatchFile(p.Dir, name):
		if _, ok := p.IgnoredGoFiles[name]; ok {
			delete(p.IgnoredGoFiles, name)
			index = true
		}
		p.GoFiles[name] = f

	default:
		if _, ok := p.GoFiles[name]; ok {
			delete(p.GoFiles, name)
		}
		p.IgnoredGoFiles[name] = f
	}

	if index && p.FindPackageFiles() {
		return c.indexFile(p, &f, fset)
	}
	return nil
}

func (c *Corpus) addFile(p *Package, name string, fset *token.FileSet) error {
	if !isGoFile(name) {
		return nil
	}
	path := filepath.Join(p.Dir, name)
	f, err := NewFile(path, c.IndexFileInfo)
	if err != nil {
		return err
	}
	index := false
	switch {
	case isGoTestFile(name):
		p.TestGoFiles[name] = f
	case c.ctxt.MatchFile(p.Dir, name):
		p.GoFiles[name] = f
		index = true
	default:
		p.IgnoredGoFiles[name] = f
	}
	if index && p.mode > FindPackageOnly {
		return c.indexFile(p, &f, fset)
	}
	return nil
}

// TODO: Rename
func (c *Corpus) indexFile(p *Package, f *File, fset *token.FileSet) error {
	switch {
	case p.FindPackageFiles():
		name, ok := parseFileName(f.Path, fset)
		if !ok {
			return nil
		}
		switch p.Name {
		case "":
			p.Name = name
		case name:
			// Ok
		default:
			var firstFile File
			for _, f := range p.GoFiles {
				firstFile = f
				break
			}
			return &MultiplePackageError{
				Dir:      p.Dir,
				Packages: []string{p.Name, name},
				Files:    []string{firstFile.Name, f.Name},
			}
		}
	case p.FindPackageName():
		if p.Name == "" {
			if name, ok := parseFileName(f.Path, fset); ok {
				p.Name = name
			}
		}
	}
	return nil
}

// NoGoError is the error used by Import to describe a directory
// containing no buildable Go source files. (It may still contain
// test files, files hidden by build tags, and so on.)
type NoGoError struct {
	Dir string
}

func (e *NoGoError) Error() string {
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
