package pkg

import (
	"encoding/json"

	"fmt"
)

func init() {
	// sanity check.
	if lastKind > 8 {
		panic("pkg: internal error lastKind > 8")
	}
}

type TypKind uint64

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

func (t TypKind) String() string {
	// Make sure lastKind is up to date.
	if t < lastKind {
		return typKindStr[t]
	}
	return typKindStr[InvalidDecl]
}

func (t TypKind) Name() string { return t.String() }

func (t TypKind) MarshalJSON() ([]byte, error) {
	return []byte(`"` + t.String() + `"`), nil
}

func (t *TypKind) UnmarshalJSON(b []byte) error {
	i, j := 0, len(b)
	if b[i] == '"' {
		i++
	}
	if b[j-1] == '"' {
		j--
	}
	*t = typKindMap[string(b[i:j])]
	return nil
}

type TypInfo uint64

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

func (x TypInfo) Kind() TypKind { return TypKind(x & 7) }
func (x TypInfo) Line() int     { return int(x >> 4 & 0xfffffff) }
func (x TypInfo) Offset() int   { return int(x >> 32) }

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
