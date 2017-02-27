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
