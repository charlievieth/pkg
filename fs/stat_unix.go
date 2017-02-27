// +build darwin linux

package fs

import (
	"os"
	"syscall"
)

func (fs *FS) stat(name string) (os.FileInfo, error) {
	var sys syscall.Stat_t
	err := syscall.Stat(name, &sys)
	if err != nil {
		return nil, &os.PathError{"stat", name, err}
	}
	var f fileStat
	fillFileStatFromSys(&f, &sys, name)
	return &f, nil
}

func (fs *FS) lstat(name string) (os.FileInfo, error) {
	var sys syscall.Stat_t
	err := syscall.Lstat(name, &sys)
	if err != nil {
		return nil, &os.PathError{"lstat", name, err}
	}
	var f fileStat
	fillFileStatFromSys(&f, &sys, name)
	return &f, nil
}
