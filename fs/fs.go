// Package fs provides file-system utilities and an implementation of
// os.FileInfo that implements the GobEncode, GobDecode, MarshalJSON and
// UnmarshalJSON interfaces.
package fs

import (
	"io"
	"io/ioutil"
	"os"
	pathpkg "path"
	"sort"
)

// Limit the number of simultaneously open files and directories.
const (
	DefaultMaxOpenFiles = 200
	DefaultMaxOpenDirs  = 50
)

// An FS provides gated access to the file system.  If maxOpenFiles or
// maxOpenDirs are not set the defaults are used.
type FS struct {
	maxOpenFiles int // max number of open files
	maxOpenDirs  int // max number of open directories
	fsOpenGate   chan struct{}
	fsDirGate    chan struct{}
}

// New, returns a new FS with maxOpenFiles and maxOpenDirs.
//
// If maxOpenFiles or maxOpenDirs are less than zero, the number of
// simultaneously open files or directories is not limited.
//
// If maxOpenFiles or maxOpenDirs are equal to zero, the default
// max open files and directories are used.
func New(maxOpenFiles, maxOpenDirs int) *FS {
	return &FS{
		maxOpenFiles: maxOpenFiles,
		maxOpenDirs:  maxOpenDirs,
		fsOpenGate:   make(chan struct{}, maxOpenFiles),
		fsDirGate:    make(chan struct{}, maxOpenDirs),
	}
}

// lazyInit, lazy initialization of FS.
func (fs *FS) lazyInit() {
	if fs.fsOpenGate == nil && fs.maxOpenFiles > -1 {
		if fs.maxOpenFiles == 0 {
			fs.maxOpenFiles = DefaultMaxOpenFiles
		}
		fs.fsOpenGate = make(chan struct{}, fs.maxOpenFiles)
	}
	if fs.fsDirGate == nil && fs.maxOpenDirs > -1 {
		if fs.maxOpenDirs == 0 {
			fs.maxOpenDirs = DefaultMaxOpenDirs
		}
		fs.fsDirGate = make(chan struct{}, fs.maxOpenDirs)
	}
}

func (fs *FS) openFileGate() {
	if fs.maxOpenFiles > -1 {
		fs.lazyInit()
		fs.fsOpenGate <- struct{}{}
	}
}

func (fs *FS) closeFileGate() {
	if fs.maxOpenFiles > 0 {
		<-fs.fsOpenGate
	}
}

func (fs *FS) openDirGate() {
	if fs.maxOpenDirs > -1 {
		fs.lazyInit()
		fs.fsDirGate <- struct{}{}
	}
}

func (fs *FS) closeDirGate() {
	if fs.maxOpenDirs > 0 {
		<-fs.fsDirGate
	}
}

// Lstat returns a os.FileInfo describing the named file.
// If the file is a symbolic link, the returned os.FileInfo
// describes the symbolic link.  Lstat makes no attempt to follow the link.
// If there is an error, it will be of type *os.PathError.
func (fs *FS) Lstat(name string) (os.FileInfo, error) {
	fi, err := os.Lstat(name)
	if err != nil {
		return nil, err
	}
	return newFileStat(fi), nil
}

// Stat returns a os.FileInfo describing the named file.
// If there is an error, it will be of type *os.PathError.
func (fs *FS) Stat(name string) (os.FileInfo, error) {
	fi, err := os.Stat(name)
	if err != nil {
		return nil, err
	}
	return newFileStat(fi), nil
}

// ReadFile reads the file named by filename and returns the contents.
func (fs *FS) ReadFile(path string) ([]byte, error) {
	fs.openFileGate()
	defer fs.closeFileGate()
	return ioutil.ReadFile(path)
}

// A fileCloser provides a ReadCloser interface to a File.
type fileCloser struct {
	f  *os.File
	fs *FS
}

// Read, reads from the underlying os.File.
func (f *fileCloser) Read(p []byte) (n int, err error) {
	return f.f.Read(p)
}

// Close, closes the underlying os.File and file gate.
func (f *fileCloser) Close() error {
	f.fs.closeFileGate()
	return f.f.Close()
}

// OpenFile, returns the file named by path for reading.
func (fs *FS) OpenFile(path string) (io.ReadCloser, error) {
	fs.openFileGate()
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	return &fileCloser{f: f, fs: fs}, nil
}

// Readdirnames reads and returns a slice of names from directory path, in
// sorted order.
func (fs *FS) Readdirnames(path string) ([]string, error) {
	fs.openDirGate()
	defer fs.closeDirGate()

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

// byName, sorts os.FileInfo by name.
type byName []os.FileInfo

func (b byName) Len() int           { return len(b) }
func (b byName) Less(i, j int) bool { return b[i].Name() < b[j].Name() }
func (b byName) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }

// Readdir reads reads the directory named by path and returns a slice of
// os.FileInfo values as would be returned by Lstat, in sorted order.
func (fs *FS) Readdir(path string) ([]os.FileInfo, error) {
	fs.openDirGate()
	defer fs.closeDirGate()

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
	sort.Sort(byName(fis))
	return fis, nil
}

// FilterFunc, returns if a file name should be included.
type FilterFunc func(string) bool

// FilterGo, is a filter func for Go source files.
func FilterGo(s string) bool {
	return len(s) >= len(".go") && s[len(s)-len(".go"):] == ".go"
}

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

// ReaddirFunc reads reads the directory named by path and returns a slice of
// os.FileInfo matched by FilterFunc fn, in sorted order.
//
// Note: Behavior is undefined if path is not absolute.
func (fs *FS) ReaddirFunc(path string, fn FilterFunc) ([]os.FileInfo, error) {
	names, err := fs.Readdirnames(path)
	names = FilterList(names, fn)
	list := make([]os.FileInfo, 0, len(names))
	for _, n := range names {
		fi, lerr := fs.Stat(pathpkg.Join(path, n))
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

// IsDir, returns if path name is a directory.
func (fs *FS) IsDir(name string) bool {
	fi, err := fs.Stat(name)
	return err == nil && fi.IsDir()
}

// IsDir, returns if path name is a file.
func (fs *FS) IsFile(name string) bool {
	fi, err := fs.Stat(name)
	return err == nil && !fi.IsDir()
}

// default FS.
var std = New(DefaultMaxOpenFiles, DefaultMaxOpenDirs)

// Lstat calls Lstat of the default FS.
func Lstat(name string) (os.FileInfo, error) {
	return std.Lstat(name)
}

// Stat calls Stat of the default FS.
func Stat(name string) (os.FileInfo, error) {
	return std.Stat(name)
}

// ReadFile reads the file named by filename using the standard FS and
// returns the contents.
func ReadFile(path string) ([]byte, error) {
	return std.ReadFile(path)
}

// OpenFile, returns the file named by path for reading using the standard FS.
func OpenFile(path string) (io.ReadCloser, error) {
	return std.OpenFile(path)
}

// Readdirnames, uses the default FS to read and return a slice of names from
// the directory f, in sorted order.
func Readdirnames(path string) ([]string, error) {
	return std.Readdirnames(path)
}

// Readdir uses the default FS to read the contents of the directory name.
func Readdir(path string) ([]os.FileInfo, error) {
	return std.Readdir(path)
}

// ReaddirFunc calls ReaddirFunc of the default FS.
func ReaddirFunc(path string, fn FilterFunc) ([]os.FileInfo, error) {
	return std.ReaddirFunc(path, fn)
}

// IsDir, returns if path name is a directory, using the default FS.
func IsDir(name string) bool {
	return std.IsDir(name)
}

// IsDir, returns if path name is a file, using the default FS.
func IsFile(name string) bool {
	return std.IsFile(name)
}

// IsPathErr, returns if error err is a *os.PathError.
func IsPathErr(err error) bool {
	_, ok := err.(*os.PathError)
	return ok
}

// SameFile, returns if os.FileInfo fi1 and fi2 have the same: name, size,
// modtime, directory mode or are both nil.
func SameFile(fi1, fi2 os.FileInfo) bool {
	if fi1 == nil {
		return fi2 == nil
	}
	return fi2 != nil &&
		fi1.ModTime() == fi2.ModTime() &&
		fi1.Size() == fi2.Size() &&
		fi1.Name() == fi2.Name() &&
		fi1.IsDir() == fi2.IsDir()
}
