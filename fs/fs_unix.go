// +build darwin linux

package fs

import "os"

func (fs *FS) readdir(dirname string) ([]os.FileInfo, error) {
	names, err := fs.Readdirnames(dirname)
	if err != nil && len(names) == 0 {
		return nil, err
	}

	fis := make([]os.FileInfo, 0, len(names))
	for _, filename := range names {
		fip, lerr := fs.lstat(dirname + "/" + filename)
		if lerr != nil {
			if os.IsNotExist(lerr) {
				continue
			}
			return fis, lerr
		}
		fis = append(fis, fip)
	}
	return fis, nil
}

// basename removes trailing slashes and the leading directory name from path name
func basename(name string) string {
	i := len(name) - 1
	// Remove trailing slashes
	for ; i > 0 && name[i] == '/'; i-- {
		name = name[:i]
	}
	// Remove leading directory name
	for i--; i >= 0; i-- {
		if name[i] == '/' {
			name = name[i+1:]
			break
		}
	}

	return name
}
