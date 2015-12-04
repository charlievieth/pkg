package pkg

import (
	"encoding/json"
	"math"
	"testing"
)

func TestTypeKindStrMap(t *testing.T) {
	if len(typKindStr) != len(typKindMap) {
		t.Fatalf("TestTypeKindStrMap: Length typKindStr: %d typKindMap: %d",
			len(typKindStr), len(typKindMap))
	}
	for i, s := range typKindStr {
		k, ok := typKindMap[s]
		if !ok {
			t.Fatalf("typKindMap: missing TypKind %q", s)
		}
		if TypKind(i) != k {
			t.Fatalf("typKindMap: bad TypKind: %q expected: %q", k, TypKind(i))
		}
	}
}

func TestTypeKindJSON(t *testing.T) {
	for i := InvalidDecl; i <= lastKind; i++ {
		b, err := json.Marshal(i)
		if err != nil {
			t.Fatal(err)
		}
		var k TypKind
		if err := json.Unmarshal(b, &k); err != nil {
			t.Fatal(err)
		}
		exp := i
		if i >= lastKind {
			exp = InvalidDecl
		}
		if k != exp {
			t.Fatalf("TestTypeKindJSON: Got %s Expected %s", k, exp)
		}
	}
}

func TestMakeTypeInfo(t *testing.T) {
	// Test TypInfo encoding limits.
	//
	// Note: no need to test TypKind limit as the package panics
	// on initialization of 'lastKind' is greater than 8.

	// Test limits
	{
		kind := lastKind - 1
		offset := math.MaxUint32
		line := math.MaxUint32 >> 4
		k := makeTypInfo(kind, offset, line)
		if k.Kind() != kind {
			t.Errorf("TypeInfo kind %v: %v", kind, k.Kind())
		}
		if k.Offset() != offset {
			t.Errorf("TypeInfo offset %v: %v", offset, k.Offset())
		}
		if k.Line() != line {
			t.Errorf("TypeInfo line %v: %v", line, k.Line())
		}
	}
	// Exceed max offset (32 bits)
	{
		kind := lastKind - 1
		line := math.MaxUint32 >> 4

		offset := math.MaxUint32
		offset++
		k := makeTypInfo(kind, offset, line)
		offset = 0

		if k.Kind() != kind {
			t.Errorf("TypeInfo kind %v: %v", kind, k.Kind())
		}
		if k.Offset() != offset {
			t.Errorf("TypeInfo offset %v: %v", offset, k.Offset())
		}
		if k.Line() != line {
			t.Errorf("TypeInfo line %v: %v", line, k.Line())
		}
	}
	// Exceed max line (28 bits)
	{
		kind := lastKind - 1
		offset := math.MaxUint32

		line := math.MaxUint32 >> 4
		line++

		k := makeTypInfo(kind, offset, line)
		line = 0

		if k.Kind() != kind {
			t.Errorf("TypeInfo kind %v: %v", kind, k.Kind())
		}
		if k.Offset() != offset {
			t.Errorf("TypeInfo offset %v: %v", offset, k.Offset())
		}
		if k.Line() != line {
			t.Errorf("TypeInfo line %v: %v", line, k.Line())
		}
	}
}

func TestTypeInfoJSON(t *testing.T) {
	kind := lastKind - 1
	offset := math.MaxUint32
	line := math.MaxUint32 >> 4
	k := makeTypInfo(kind, offset, line)

	b, err := json.Marshal(k)
	if err != nil {
		t.Fatal(err)
	}
	var v TypInfo
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatal(err)
	}
	if v != k {
		t.Fatalf("TestTypeInfoJSON: Expected %v Got %v", k, v)
	}
}
