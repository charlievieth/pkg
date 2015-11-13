// Package fs provides file-system utilities.
package fs

import (
	"bytes"
	"encoding/gob"
	"io/ioutil"
	"os"
	pathpkg "path"
	"sort"
	"time"
)

// Limit the number of simultaneously open files and directories.
const (
	maxOpenFiles = 200
	maxOpenDirs  = 50
)

var fsOpenGate = make(chan struct{}, maxOpenFiles)
var fsDirGate = make(chan struct{}, maxOpenDirs)

func init() {
	gob.Register(&fileStat{})
}

type fileStat struct {
	name    string
	size    int64
	mode    os.FileMode
	modTime time.Time
	isDir   bool
}

func newFileStat(fi os.FileInfo) *fileStat {
	if fi == nil {
		return nil
	}
	return &fileStat{
		name:    fi.Name(),
		size:    fi.Size(),
		mode:    fi.Mode(),
		modTime: fi.ModTime(),
		isDir:   fi.IsDir(),
	}
}

func (fs *fileStat) Name() string       { return fs.name }
func (fs *fileStat) Size() int64        { return fs.size }
func (fs *fileStat) Mode() os.FileMode  { return fs.mode }
func (fs *fileStat) ModTime() time.Time { return fs.modTime }
func (fs *fileStat) IsDir() bool        { return fs.isDir }
func (fs *fileStat) Sys() interface{}   { return nil }

func (f *fileStat) GobDecode(b []byte) error {
	var v struct {
		Name    string
		Size    int64
		Mode    os.FileMode
		ModTime time.Time
		IsDir   bool
	}
	if err := gob.NewDecoder(bytes.NewReader(b)).Decode(&v); err != nil {
		return err
	}
	f.name = v.Name
	f.size = v.Size
	f.mode = v.Mode
	f.modTime = v.ModTime
	f.isDir = v.IsDir
	return nil
}

func (f *fileStat) GobEncode() ([]byte, error) {
	v := struct {
		Name    string
		Size    int64
		Mode    os.FileMode
		ModTime time.Time
		IsDir   bool
	}{
		f.name,
		f.size,
		f.mode,
		f.modTime,
		f.isDir,
	}
	buf := new(bytes.Buffer)
	if err := gob.NewEncoder(buf).Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
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

func IsDir(name string) bool {
	fs, err := Stat(name)
	return err == nil && fs.IsDir()
}

func IsFile(name string) bool {
	fs, err := Stat(name)
	return err == nil && !fs.IsDir()
}

func IsPathErr(err error) bool {
	_, ok := err.(*os.PathError)
	return ok
}
