package fs

import (
	"testing"
)

// Simple initialization test.
func TestFSInit(t *testing.T) {
	// Make sure FS initializes itself and does not panic.
	defer func() {
		if e := recover(); e != nil {
			t.Fatalf("PANIC FS Init: %+v", e)
		}
	}()

	// Test defaults
	var fs FS
	fs.Readdir(".")
	if fs.maxOpenFiles != DefaultMaxOpenFiles {
		t.Errorf("FS Init maxOpenFiles: exp: %d got: %d", DefaultMaxOpenFiles, fs.maxOpenFiles)
	}
	if fs.maxOpenDirs != DefaultMaxOpenDirs {
		t.Errorf("FS Init maxOpenDirs: exp: %d got: %d", DefaultMaxOpenDirs, fs.maxOpenDirs)
	}

	// Test value
	max := 1
	fs = FS{
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

	// Test -1
	fs = FS{
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
