// +build !darwin,!linux

package fs

import "os"

func (fs *FS) readdir(path string) ([]os.FileInfo, error) {
	fs.openDirGate()
	defer fs.closeDirGate()

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	names, err := f.Readdir(-1)
	f.Close()
	if err != nil && len(names) == 0 {
		return nil, err
	}
	fis := make([]os.FileInfo, len(names))
	for i, n := range names {
		fis[i] = newFileStat(n)
	}
	return fis, nil
}

// this is mostly for Windows where filtering on the results of Readdir is
// significantly faster than filtering on the results of Readdirnames and
// calling Lstat() in each file.
func (fs *FS) readdirfunc(dirname string, fn FilterFunc) ([]os.FileInfo, error) {
	fis, err := fs.Readdir(dirname)
	if err != nil && len(fis) == 0 {
		return nil, err
	}
	n := 0
	for i := range fis {
		if fn(fis[i].Name()) {
			fis[n] = fis[i]
			n++
		}
	}
	return fis[:n], nil
}
