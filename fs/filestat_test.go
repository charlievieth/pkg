package fs

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"
)

type statField struct {
	S    string
	Stat *fileStat
	N    int64
	Ok   bool
}

// Sanity check to ensure fileStat and fileStatExt fields match.
func TestFileStatFields(t *testing.T) {
	ff := structFields(fileStat{}, t)
	ef := structFields(fileStatExt{}, t)
	if len(ff) != len(ef) {
		t.Fatalf("Fields count mismatch: fileStat: %d fileStatExt: %d", len(ff), len(ef))
	}
	for n, ft := range ff {
		et, ok := ef[n]
		if !ok {
			t.Fatalf("Field (%s): not in fileStatExt", n)
		}
		if ft.Type.Kind() != et.Type.Kind() {
			t.Fatalf("Field Type Kind (%s): fileStat: %s fileStatExt: %s", n,
				ft.Type.Kind(), et.Type.Kind())
		}
	}
}

func structFields(v interface{}, t *testing.T) map[string]reflect.StructField {
	defer func() {
		if e := recover(); e != nil {
			t.Fatalf("PANIC StructFields: %v", e)
		}
	}()
	fields := make(map[string]reflect.StructField)
	typ := reflect.TypeOf(v)
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		fields[strings.ToLower(f.Name)] = f
	}
	return fields
}

func TestEncode(t *testing.T) {
	// Simple
	{
		enc := &fileStat{
			name:    "a",
			size:    123,
			mode:    123,
			modTime: time.Now(),
		}
		// Gob
		dec := &fileStat{}
		encodeDecodeGob(enc, dec, t)
		compareFileStats(enc, dec, t)
		// JSON
		dec = &fileStat{}
		encodeDecodeJSON(enc, dec, t)
		compareFileStats(enc, dec, t)
	}
	// Struct field
	{
		enc := &statField{
			S: "Struct",
			Stat: &fileStat{
				name:    "a",
				size:    123,
				mode:    123,
				modTime: time.Now(),
			},
			N:  math.MaxInt64,
			Ok: true,
		}
		// Gob
		dec := &statField{}
		encodeDecodeGob(enc, dec, t)
		compareStatFields(enc, dec, t)
		// JSON
		dec = &statField{}
		encodeDecodeJSON(enc, dec, t)
		compareStatFields(enc, dec, t)
	}
}

func encodeDecodeJSON(enc, dec interface{}, t *testing.T) {
	buf := new(bytes.Buffer)
	if err := json.NewEncoder(buf).Encode(enc); err != nil {
		t.Errorf("JSON Encode (%+v): %s", enc, err)
	}
	if err := json.NewDecoder(buf).Decode(dec); err != nil {
		t.Errorf("JSON Decode (%+v): %s", enc, err)
	}
}

func encodeDecodeGob(enc, dec interface{}, t *testing.T) {
	buf := new(bytes.Buffer)
	if err := gob.NewEncoder(buf).Encode(enc); err != nil {
		t.Errorf("Gob Encode (%+v): %s", enc, err)
	}
	if err := gob.NewDecoder(buf).Decode(dec); err != nil {
		t.Errorf("Gob Decode (%+v): %s", enc, err)
	}
}

func compareStatFields(f1, f2 *statField, t *testing.T) {
	switch {
	case f1.S != f2.S:
		t.Errorf("StatField (S): F1 (%+v) F2 (%+v)", f1, f2)
	case f1.N != f2.N:
		t.Errorf("StatField (N): F1 (%+v) F2 (%+v)", f1, f2)
	case f1.Ok != f2.Ok:
		t.Errorf("StatField (Ok): F1 (%+v) F2 (%+v)", f1, f2)
	}
	compareFileStats(f1.Stat, f2.Stat, t)
}

func compareFileStats(f1, f2 *fileStat, t *testing.T) {
	switch {
	case f1.name != f2.name:
		t.Errorf("FileStat (name): F1 (%+v) F2: (%+v)", f1, f2)
	case f1.size != f2.size:
		t.Errorf("FileStat (size): F1 (%+v) F2: (%+v)", f1, f2)
	case f1.mode != f2.mode:
		t.Errorf("FileStat (mode): F1 (%+v) F2: (%+v)", f1, f2)
	case !f1.modTime.Equal(f2.modTime):
		t.Errorf("FileStat (mode): F1 (%+v) F2: (%+v)", f1, f2)
	}
}
