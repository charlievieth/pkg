package fs

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"os"
	"time"
)

func init() {
	gob.Register(&fileStat{})
}

// A fileStat is the implementation of FileInfo returned by Stat and Lstat,
// that implements the GobEncode, GobDecode, MarshalJSON and UnmarshalJSON.
// Sys() always returns nil.
type fileStat struct {
	name    string
	size    int64
	mode    os.FileMode
	modTime time.Time
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
	}
}

// Name, base name of the file
func (fs *fileStat) Name() string { return fs.name }

// Size, length in bytes for regular files; system-dependent for others
func (fs *fileStat) Size() int64 { return fs.size }

// Mode, file mode bits
func (fs *fileStat) Mode() os.FileMode { return fs.mode }

// ModTime, modification time
func (fs *fileStat) ModTime() time.Time { return fs.modTime }

// IsDir, abbreviation for Mode().IsDir()
func (fs *fileStat) IsDir() bool { return fs.mode.IsDir() }

// Sys, underlying data source, unlike os.FileInfo nil is always returned.
func (fs *fileStat) Sys() interface{} { return nil }

// A fileStatExt is the encoded form of fileStat (exported fields).
type fileStatExt struct {
	Name    string
	Size    int64
	Mode    os.FileMode
	ModTime time.Time
}

// ext, returns the fileStatExt representation of f for encoding.
func (f *fileStat) ext() *fileStatExt {
	return &fileStatExt{
		Name:    f.name,
		Size:    f.size,
		Mode:    f.mode,
		ModTime: f.modTime,
	}
}

// set, sets the fields of f to those fileStatExt e, used for decoding.
func (f *fileStat) set(e *fileStatExt) {
	f.name = e.Name
	f.size = e.Size
	f.mode = e.Mode
	f.modTime = e.ModTime
}

func (f *fileStat) GobDecode(b []byte) error {
	var v fileStatExt
	r := bytes.NewReader(b)
	if err := gob.NewDecoder(r).Decode(&v); err != nil {
		return err
	}
	f.set(&v)
	return nil
}

func (f *fileStat) GobEncode() ([]byte, error) {
	buf := new(bytes.Buffer)
	err := gob.NewEncoder(buf).Encode(f.ext())
	return buf.Bytes(), err
}

func (f *fileStat) MarshalJSON() ([]byte, error) {
	return json.Marshal(f.ext())
}

func (f *fileStat) UnmarshalJSON(b []byte) error {
	var v fileStatExt
	err := json.Unmarshal(b, &v)
	f.set(&v)
	return err
}
