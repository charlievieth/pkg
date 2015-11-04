package pkg

import (
	"errors"
	"fmt"
	"go/token"
	"os"
	"path/filepath"
	"sort"
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
		f.Info = fi
	}
	return f, nil
}

func (f *File) Valid() bool {
	return f.Name != "" && f.Path != ""
}

func (f File) String() string {
	// Here to make debugging a little easier.
	const s = "{Name:%s Path:%s Info:{Name:%s Size:%d Mode:%s ModTime:%s IsDir:%v}}"
	return fmt.Sprintf(s, f.Name, f.Path, f.Info.Name(), f.Info.Size(),
		f.Info.Mode(), f.Info.ModTime(), f.Info.IsDir())
}

type ByFileName []File

func (f ByFileName) Len() int           { return len(f) }
func (f ByFileName) Less(i, j int) bool { return f[i].Name < f[j].Name }
func (f ByFileName) Swap(i, j int)      { f[i].Name, f[j].Name = f[j].Name, f[i].Name }

type FileMap map[string]File

func (m FileMap) Files() []File {
	s := m.appendFiles(make([]File, 0, len(m)))
	sort.Sort(ByFileName(s))
	return s
}

func (m FileMap) FileNames() []string {
	s := m.appendFileNames(make([]string, 0, len(m)))
	sort.Strings(s)
	return s
}

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

// TODO (CEV): Map files by type (map[Type]FileMap)

type Package struct {
	Dir            string      // Directory path "$GOROOT/src/net/http"
	Name           string      // Package name "http"
	ImportPath     string      // Import path of package "net/http"
	Root           string      // Root of Go tree where this package lives
	Goroot         bool        // Package found in Go root
	GoFiles        FileMap     // .go source files (excluding TestGoFiles and IgnoredGoFiles)
	IgnoredGoFiles FileMap     // .go source files ignored for this build
	TestGoFiles    FileMap     // _test.go files in package
	Info           os.FileInfo // File info as of last update
	mode           ImportMode  // ImportMode used when created
	err            error       // Either NoGoError of MultiplePackageError
}

func (p *Package) FindPackageName() bool {
	return p.mode&FindPackageName != 0
}

func (p *Package) FindPackageFiles() bool {
	return p.mode&FindPackageFiles != 0
}

// Mode, returns the ImportMode used to parse the package.
func (p *Package) Mode() ImportMode {
	return p.mode
}

// Error, returns either NoGoError or MultiplePackageError.
func (p *Package) Error() error {
	return p.err
}

func (p *Package) LookupFile(name string) (File, bool) {
	if f, ok := p.GoFiles[name]; ok {
		return f, ok
	}
	if f, ok := p.IgnoredGoFiles[name]; ok {
		return f, ok
	}
	if f, ok := p.TestGoFiles[name]; ok {
		return f, ok
	}
	return File{}, false
}

// IsCommand reports whether the package is considered a command to be installed
// (not just a library). Packages named "main" are treated as commands.
func (p *Package) IsCommand() bool {
	return p.Name == "main"
}

func (p *Package) SrcFiles() []string {
	return p.GoFiles.FileNames()
}

// fileNames, returns the names of all Go src files.
func (p *Package) fileNames() []string {
	// TODO (CEV): This map setup is becoming a real pain - fix!
	n := len(p.GoFiles) + len(p.TestGoFiles) + len(p.IgnoredGoFiles)
	s := make([]string, 0, n)
	s = p.GoFiles.appendFileNames(s)
	s = p.TestGoFiles.appendFileNames(s)
	s = p.IgnoredGoFiles.appendFileNames(s)
	return s
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

func (p *Package) deleteFile(name string) {
	delete(p.GoFiles, name)
	delete(p.IgnoredGoFiles, name)
	delete(p.TestGoFiles, name)
}

func (p *Package) isPkgDir() bool {
	return len(p.GoFiles) != 0 ||
		len(p.TestGoFiles) != 0 ||
		len(p.IgnoredGoFiles) != 0
}

// findPkgName, attempts to find the pkg name.  If there are no buildable
// Gofiles we don't parse any package names, this parses ignored and test
// files until a name is found.
func (p *Package) findPkgName(fset *token.FileSet) {
	if !p.isPkgDir() {
		return
	}
	for _, f := range p.IgnoredGoFiles {
		if n, ok := parseFileName(fset, f.Path); ok {
			p.Name = n
			return
		}
	}
	for _, f := range p.TestGoFiles {
		if n, ok := parseFileName(fset, f.Path); ok {
			p.Name = n
			return
		}
	}
}

// removeNotSeen, removes any files not listed in seen.
func (p *Package) removeNotSeen(seen []string) {
	m := make(map[string]bool, len(seen))
	for _, s := range seen {
		m[s] = true
	}
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
		if hasRoot(dir, srcDir) {
			p.ImportPath = trimPathPrefix(dir, srcDir)
			p.Root = filepath.Dir(srcDir)
			p.Goroot = hasRoot(dir, c.ctxt.GOROOT())
			break
		}
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

// WARN: Dev only
func (c *Corpus) updatePackageFast(p *Package) (*Package, error) {
	fi, err := os.Stat(p.Dir)
	if err != nil {
		return nil, err
	}
	fset := token.NewFileSet()
	return c.updatePackage(p, fi, fset, nil)
}

// TODO: Organize args
func (c *Corpus) updatePackage(p *Package, fi os.FileInfo, fset *token.FileSet,
	names []string) (*Package, error) {

	if !fi.IsDir() {
		return nil, errors.New("pkg: invalid Package path: " + p.Dir)
	}

	p.mode = c.PackageMode
	// Unless we are indexing fileinfo return, reading
	// in the dirnames on each update is very slow.
	if len(names) == 0 && !c.IndexFileInfo {
		p.Info = fi
		return p, nil
	}

	// If the directory did not change we can skip reading
	// the directory names for speedup of 4x.
	var err error
	if sameFile(p.Info, fi) && names == nil {
		names = p.fileNames()
	} else {
		names, err = completeDirnames(p.Dir, names)
		if err != nil {
			return nil, err
		}
	}
	p.Info = fi

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
	p.removeNotSeen(names)
	if !p.isPkgDir() {
		return p, pkgErr
	}
	if p.Name == "" {
		// Attempt to find the package name.
		p.findPkgName(fset)
		pkgErr = &NoBuildableGoError{Dir: p.Dir}
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
		if err != nil || fi.IsDir() {
			p.deleteFile(name)
			return err
		}
		if sameFile(f.Info, fi) {
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
	if index {
		return c.indexFile(p, &f, fset)
	}
	return nil
}

// TODO: Rename
func (c *Corpus) indexFile(p *Package, f *File, fset *token.FileSet) error {
	switch {
	case p.FindPackageFiles():
		name, ok := parseFileName(fset, f.Path)
		if !ok {
			return nil
		}
		switch p.Name {
		case "":
			p.Name = name
		case name:
			// Ok
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
			if name, ok := parseFileName(fset, f.Path); ok {
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

type PackageIndexer struct {
	c        *Corpus
	fset     *token.FileSet
	mode     ImportMode
	packages map[string]map[string]*Package // "$GOPATH/src" => "net/http" => Package
	mu       sync.RWMutex
}

func (x *PackageIndexer) lookupPath(path string) *Package {
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

func (x *PackageIndexer) removePackage(p *Package) {
	if x.packages == nil || x.packages[p.Root] == nil {
		return
	}
	x.mu.Lock()
	delete(x.packages[p.Root], p.ImportPath)
	x.mu.Unlock()
}

func (x *PackageIndexer) visitDirectory(dir *Directory, names []string) *Package {
	if p := x.lookupPath(dir.Path); p != nil {
		if err := x.updatePackage(p, dir.Info, names); err != nil {
			x.removePackage(p)
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

func (x *PackageIndexer) importPackage(dir string, fi os.FileInfo, names []string) (*Package, error) {
	if !hasGoFiles(names) {
		return nil, &NoGoError{Dir: dir}
	}
	p := x.newPackage(dir, fi)
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

func (x *PackageIndexer) updatePackage(p *Package, fi os.FileInfo, names []string) error {

	if !fi.IsDir() {
		return errors.New("pkg: invalid Package path: " + p.Dir)
	}

	p.mode = x.mode
	if !x.c.IndexFileInfo && p.Name != "" {
		p.Info = fi
		return nil
	}

	// If the directory did not change we can skip reading
	// the directory names for speedup of 4x.
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
	if p.Name == "" {
		// Attempt to find the package name.
		x.findPkgName(p)
		if p.err == nil {
			p.err = &NoBuildableGoError{Dir: p.Dir}
		}
	}
	return nil
}

// readdirnames
func (x *PackageIndexer) readdirnames(p *Package, names []string, usePkgFiles bool) ([]string, error) {
	if names != nil {
		return names, nil
	}
	if usePkgFiles && p != nil {
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
	if x.c.IndexFileInfo {
		fi, err := os.Stat(f.Path)
		if err != nil || fi.IsDir() {
			p.deleteFile(name)
			return err
		}
		if sameFile(f.Info, fi) {
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

	case x.c.ctxt.MatchFile(p.Dir, name):
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
		return x.indexFile(p, &f)
	}
	return nil
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

func (x *PackageIndexer) newPackage(dir string, fi os.FileInfo) *Package {
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
	return p
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
	index := false
	switch {
	case isGoTestFile(name):
		p.TestGoFiles[name] = f
	case x.c.ctxt.MatchFile(p.Dir, name):
		p.GoFiles[name] = f
		index = true
	default:
		p.IgnoredGoFiles[name] = f
	}
	if index {
		return x.indexFile(p, &f)
	}
	return nil
}

func (x *PackageIndexer) indexFile(p *Package, f *File) error {
	switch {
	case p.FindPackageFiles():
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
