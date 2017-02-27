// +build !darwin,!linux

package fs

import (
	"os"
)

func (fs *FS) lstat(name string) (os.FileInfo, error) {
	fi, err := os.Lstat(name)
	if err != nil {
		return nil, err
	}
	return newFileStat(fi), nil
}

func (fs *FS) stat(name string) (os.FileInfo, error) {
	fi, err := os.Stat(name)
	if err != nil {
		return nil, err
	}
	return newFileStat(fi), nil
}
