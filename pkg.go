package pkg

import (
	"errors"
	"fmt"
	"go/token"
	"os"
	"path/filepath"
	"sort"
)

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

// A Pak is a abbreviated Package.
type Pak struct {
	Dir        string // Directory path "$GOROOT/src/net/http"
	Name       string // Package name "http"
	ImportPath string // Import path of package "net/http"
}

// A Package describes a Go package or command.
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

func (p *Package) Pak() Pak {
	return Pak{Dir: p.Dir, Name: p.Name, ImportPath: p.ImportPath}
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

func (p *Package) IsValid() bool {
	return p.Name != "" && p.isPkgDir()
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
