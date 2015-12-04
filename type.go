package pkg

import (
	"encoding/json"
	"errors"
	"fmt"
)

func init() {
	// sanity check.
	if lastKind > 8 {
		panic("pkg: internal error lastKind > 8")
	}
}

// TypKind describes the kind of a Go identifier.
type TypKind uint32

const (
	InvalidDecl TypKind = iota
	ConstDecl
	VarDecl
	TypeDecl
	FuncDecl
	MethodDecl
	InterfaceDecl

	// The last TypKind *must* be less than or equal to 8.
	lastKind
)

var typKindStr = [...]string{
	"InvalidDecl",
	"ConstDecl",
	"VarDecl",
	"TypeDecl",
	"FuncDecl",
	"MethodDecl",
	"InterfaceDecl",
}

var typKindMap = map[string]TypKind{
	"InvalidDecl":   InvalidDecl,
	"ConstDecl":     ConstDecl,
	"VarDecl":       VarDecl,
	"TypeDecl":      TypeDecl,
	"FuncDecl":      FuncDecl,
	"MethodDecl":    MethodDecl,
	"InterfaceDecl": InterfaceDecl,
}

// String, returns the string representation of t.
func (t TypKind) String() string {
	// Make sure lastKind is up to date.
	if t.IsValid() {
		return typKindStr[t]
	}
	return typKindStr[InvalidDecl]
}

// Name, returns the name (string representation) of t.
func (t TypKind) Name() string { return t.String() }

// IsValid, returns if t is a valid TypKind.
func (t TypKind) IsValid() bool { return t < lastKind }

func (t TypKind) MarshalJSON() ([]byte, error) {
	return []byte(`"` + t.String() + `"`), nil
}

func (t *TypKind) UnmarshalJSON(b []byte) (err error) {
	i, j := 0, len(b)
	if b[i] == '"' {
		i++
	}
	if b[j-1] == '"' {
		j--
	}
	if typ, ok := typKindMap[string(b[i:j])]; ok {
		*t = typ
	} else {
		err = errors.New(`pkg: invalid TypKind "` + string(string(b[i:j])) + `"`)
	}
	return err
}

// A TypeInfo value describes a particular identifier spot in a given file.
// It encodes three values: the TypeKind, and the file line and offset.
//
// The following encoding is used:
//
//   bits    64     32    4    1
//   value     [offset|line|kind]
//
// TODO (CEV): Add line offset.
type TypInfo uint64

// makeTypInfo makes a TypeInfo.
func makeTypInfo(kind TypKind, offset, line int) TypInfo {
	x := TypInfo(offset) << 32
	if int(x>>32) != offset {
		x = 0
	}
	x |= TypInfo(line) << 4
	if int(x>>4&0xfffffff) != line {
		x &^= 0xfffffff
	}
	x |= TypInfo(kind)
	return x
}

func (t TypInfo) Kind() TypKind { return TypKind(t & 7) }
func (t TypInfo) Line() int     { return int(t >> 4 & 0xfffffff) }
func (t TypInfo) Offset() int   { return int(t >> 32) }

func (t TypInfo) String() string {
	return fmt.Sprintf("{Kind:%s Offset:%d Line:%d}", t.Kind().String(),
		t.Offset(), t.Line())
}

func (t TypInfo) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Kind   TypKind
		Line   int
		Offset int
	}{
		t.Kind(),
		t.Line(),
		t.Offset(),
	})
}

func (t *TypInfo) UnmarshalJSON(b []byte) error {
	var v struct {
		Kind   TypKind
		Line   int
		Offset int
	}
	err := json.Unmarshal(b, &v)
	*t = makeTypInfo(v.Kind, v.Offset, v.Line)
	return err
}
