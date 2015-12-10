package fs

import (
	"os"
	"reflect"
	"testing"
	"time"
)

// Simple initialization test.
func TestFSInit(t *testing.T) {
	// Make sure FS initializes itself and does not panic.
	defer func() {
		if e := recover(); e != nil {
			t.Fatalf("FS Init Panic: %v", e)
		}
	}()

	// Test defaults
	{
		var fs FS
		fs.Readdir(".")
		if fs.maxOpenFiles != DefaultMaxOpenFiles {
			t.Errorf("FS Init maxOpenFiles: exp: %d got: %d", DefaultMaxOpenFiles, fs.maxOpenFiles)
		}
		if fs.maxOpenDirs != DefaultMaxOpenDirs {
			t.Errorf("FS Init maxOpenDirs: exp: %d got: %d", DefaultMaxOpenDirs, fs.maxOpenDirs)
		}
	}

	// Test value
	{
		max := 1
		fs := FS{
			maxOpenFiles: max,
			maxOpenDirs:  max,
		}
		fs.Readdir(".")
		if fs.maxOpenFiles != max {
			t.Errorf("FS Init maxOpenFiles: exp: %d got: %d", max, fs.maxOpenFiles)
		}
		if fs.maxOpenDirs != max {
			t.Errorf("FS Init maxOpenDirs: exp: %d got: %d", max, fs.maxOpenDirs)
		}
	}

	// Test -1
	{
		fs := FS{
			maxOpenFiles: -1,
			maxOpenDirs:  -1,
		}
		fs.Readdir(".")
		if fs.maxOpenFiles != -1 {
			t.Errorf("FS Init maxOpenFiles: exp: %d got: %d", -1, fs.maxOpenFiles)
		}
		if fs.maxOpenDirs != -1 {
			t.Errorf("FS Init maxOpenDirs: exp: %d got: %d", -1, fs.maxOpenDirs)
		}
		// Make sure gates are not initialized for -1.
		if fs.fsOpenGate != nil {
			t.Errorf("FS Init: non-nil %s", "fsOpenGate")
		}
		if fs.fsDirGate != nil {
			t.Errorf("FS Init: non-nil %s", "fsDirGate")
		}
	}
}

var sameFileTests = []struct {
	fi1  os.FileInfo
	fi2  os.FileInfo
	same bool
}{
	{ // 0: Same fileStats
		fi1: &fileStat{
			name:    "a",
			size:    1,
			mode:    os.ModeDir,
			modTime: time.Unix(0, 0),
		},
		fi2: &fileStat{
			name:    "a",
			size:    1,
			mode:    os.ModeDir,
			modTime: time.Unix(0, 0),
		},
		same: true,
	},
	{ // 1: Name
		fi1: &fileStat{
			name:    "a",
			size:    1,
			mode:    os.ModeDir,
			modTime: time.Unix(0, 0),
		},
		fi2: &fileStat{
			name:    "b", // name
			size:    1,
			mode:    os.ModeDir,
			modTime: time.Unix(0, 0),
		},
		same: false,
	},
	{ // 2: Size
		fi1: &fileStat{
			name:    "a",
			size:    1,
			mode:    os.ModeDir,
			modTime: time.Unix(0, 0),
		},
		fi2: &fileStat{
			name:    "a",
			size:    2, // size
			mode:    os.ModeDir,
			modTime: time.Unix(0, 0),
		},
		same: false,
	},
	{ // 3: Mode
		fi1: &fileStat{
			name:    "a",
			size:    1,
			mode:    os.ModeDir,
			modTime: time.Unix(0, 0),
		},
		fi2: &fileStat{
			name:    "a",
			size:    1,
			mode:    0, // mode
			modTime: time.Unix(0, 0),
		},
		same: false,
	},
	{ // 4: Mod Time
		fi1: &fileStat{
			name:    "a",
			size:    1,
			mode:    os.ModeDir,
			modTime: time.Unix(0, 0),
		},
		fi2: &fileStat{
			name:    "a",
			size:    1,
			mode:    os.ModeDir,
			modTime: time.Unix(0, 1), // modTime
		},
		same: false,
	},
}

func TestSameFile(t *testing.T) {
	for i, x := range sameFileTests {
		same := SameFile(x.fi1, x.fi2)
		if same != x.same {
			t.Errorf("SameFile (%v) (%+v): Exp (%v) Got (%v)", i, x, x.same, same)
		}
	}
}

func TestSameNilFile(t *testing.T) {
	// nil should equal nil
	if ok := SameFile(nil, nil); !ok {
		t.Errorf("SameFile (%v) (%v): %v", nil, nil, ok)
	}

	// Make sure SameFile does not panic.
	defer func() {
		if e := recover(); e != nil {
			t.Fatalf("SameFile Panic: %v", e)
		}
	}()
	fi := new(fileStat)
	if ok := SameFile(fi, nil); ok {
		t.Errorf("SameFile (%v) (%v): %v", nil, nil, ok)
	}
	if ok := SameFile(nil, fi); ok {
		t.Errorf("SameFile (%v) (%v): %v", nil, nil, ok)
	}
}

func TestFilterGo(t *testing.T) {
	exp := []string{
		"a.go",
		"b.go",
		"c.go",
	}
	bad := []string{
		"A.no",
		"B.no",
		"C.no",
	}
	names := append(exp, bad...)
	list := FilterList(names, FilterGo)
	if len(list) != len(exp) {
		t.Errorf("FilterGo len: Exp (%v) Got (%v)", len(exp), len(list))
	}
	for i := 0; i < len(exp); i++ {
		if exp[i] != list[i] {
			t.Errorf("FilterGo len: Exp (%v) Got (%v)", exp[i], list[i])
		}
	}
}

func TestFileCloser(t *testing.T) {
	// os file: This is the control
	fo, err := os.Open("fs_test.go")
	if err != nil {
		t.Fatalf("FileCloser error opening file: %v", err)
	}

	// fs file: Make sure read ops match 'fo'
	fs, err := OpenFile("fs_test.go")
	if err != nil {
		t.Fatalf("FileCloser error opening file: %v", err)
	}

	b1 := make([]byte, 16)
	n1, err := fo.Read(b1)
	if err != nil {
		t.Fatalf("FileCloser error reading file: %v", err)
	}

	b2 := make([]byte, 16)
	n2, err := fs.Read(b2)
	if err != nil {
		t.Fatalf("FileCloser error reading file: %v", err)
	}

	if n1 != n2 {
		t.Errorf("FileCloser read length: Exp (%v) Got (%v)", n1, n2)
	}
	if !reflect.DeepEqual(b1, b2) {
		t.Error("FileCloser: bad read")
	}

	err1 := fo.Close()
	err2 := fs.Close()
	if err1 != err2 {
		t.Errorf("FileCloser Close error: Exp (%v) Got (%v)", n1, n2)
	}
}
