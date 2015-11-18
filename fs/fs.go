// Package fs provides file-system utilities and an implementation of
// os.FileInfo that implements the GobEncode, GobDecode, MarshalJSON and
// UnmarshalJSON interfaces.
package fs

import (
	"io/ioutil"
	"os"
	pathpkg "path"
	"sort"
)

type FS struct {
	maxOpenFiles int
	maxOpenDirs  int
	fsOpenGate   chan struct{}
	fsDirGate    chan struct{}
}

func New(maxOpenFiles, maxOpenDirs int) *FS {
	return &FS{
		maxOpenFiles: maxOpenFiles,
		maxOpenDirs:  maxOpenDirs,
		fsOpenGate:   make(chan struct{}, maxOpenFiles),
		fsDirGate:    make(chan struct{}, maxOpenDirs),
	}
}

func (f *FS) lazyInit() {
	if f.fsOpenGate == nil {
		if f.maxOpenFiles <= 0 {
			f.maxOpenFiles = DefaultMaxOpenFiles
		}
		if f.maxOpenDirs <= 0 {
			f.maxOpenDirs = DefaultMaxOpenDirs
		}
		f.fsOpenGate = make(chan struct{}, f.maxOpenFiles)
		f.fsDirGate = make(chan struct{}, f.maxOpenDirs)
	}
}

// Limit the number of simultaneously open files and directories.
const (
	DefaultMaxOpenFiles = 200
	DefaultMaxOpenDirs  = 50
)

var fsOpenGate = make(chan struct{}, DefaultMaxOpenFiles)
var fsDirGate = make(chan struct{}, DefaultMaxOpenDirs)
var std = New(DefaultMaxOpenFiles, DefaultMaxOpenDirs)

func (fs *FS) Lstat(name string) (os.FileInfo, error) {
	fi, err := os.Lstat(name)
	if err != nil {
		return nil, err
	}
	return newFileStat(fi), nil
}

func Lstat(name string) (os.FileInfo, error) {
	fi, err := os.Lstat(name)
	if err != nil {
		return nil, err
	}
	return newFileStat(fi), nil
}

func Stat(name string) (os.FileInfo, error) {
	fi, err := os.Stat(name)
	if err != nil {
		return nil, err
	}
	return newFileStat(fi), nil
}

func ReadFile(path string) ([]byte, error) {
	fsOpenGate <- struct{}{}
	defer func() { <-fsOpenGate }()
	return ioutil.ReadFile(path)
}

// Readdirnames reads and returns a slice of names from the directory f, in
// sorted order.
func Readdirnames(path string) ([]string, error) {
	fsDirGate <- struct{}{}
	defer func() { <-fsDirGate }()

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	names, err := f.Readdirnames(-1)
	f.Close()
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	return names, nil
}

// Readdir reads the contents of the directory associated with file and returns
// a slice of FileInfo values as would be returned by Lstat, in directory order.
func Readdir(path string) ([]os.FileInfo, error) {
	fsDirGate <- struct{}{}
	defer func() { <-fsDirGate }()

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	names, err := f.Readdir(-1)
	f.Close()
	if err != nil {
		return nil, err
	}
	fis := make([]os.FileInfo, len(names))
	for i, n := range names {
		fis[i] = newFileStat(n)
	}
	return fis, nil
}

type FilterFunc func(string) bool

// FilterList, returns all of the members of list that satisfy fn().
func FilterList(list []string, fn FilterFunc) []string {
	n := 0
	for i := 0; i < len(list); i++ {
		if fn(list[i]) {
			list[n] = list[i]
			n++
		}
	}
	return list[:n]
}

// FilterGo, is a filter func for Go source files.
func FilterGo(s string) bool {
	return len(s) >= len(".go") && s[len(s)-len(".go"):] == ".go"
}

// StatFunc, note path must be absolute.
func StatFunc(path string, fn FilterFunc) ([]os.FileInfo, error) {
	names, err := Readdirnames(path)
	names = FilterList(names, fn)
	list := make([]os.FileInfo, 0, len(names))
	for _, n := range names {
		fi, lerr := Stat(pathpkg.Join(path, n))
		if os.IsNotExist(lerr) {
			continue
		}
		if lerr != nil {
			return list, lerr
		}
		list = append(list, fi)
	}
	return list, err
}

// SameFile, returns if os.FileInfo fi1 and fi2 have the same: name, size,
// modtime, directory mode or are both nil.
func SameFile(fi1, fi2 os.FileInfo) bool {
	if fi1 == nil {
		if fi2 == nil {
			return true
		}
		return false
	}
	return fi1.ModTime() == fi2.ModTime() &&
		fi1.Size() == fi2.Size() &&
		fi1.Name() == fi2.Name() &&
		fi1.IsDir() == fi2.IsDir()
}

// IsDir, returns if path name is a directory.
func IsDir(name string) bool {
	fs, err := Stat(name)
	return err == nil && fs.IsDir()
}

// IsDir, returns if path name is a file.
func IsFile(name string) bool {
	fs, err := Stat(name)
	return err == nil && !fs.IsDir()
}

// IsPathErr, returns if error err is a *os.PathError.
func IsPathErr(err error) bool {
	_, ok := err.(*os.PathError)
	return ok
}
