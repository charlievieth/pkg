package pkg

import (
	"testing"
)

var identNameTests = []struct {
	typ TypKind
	in  string
	out string
}{
	{FuncDecl, "A", "A"},
	// Wrong, but should not be changed.
	{FuncDecl, "A.A", "A.A"},

	{MethodDecl, "A.A", "A"},
	{InterfaceDecl, "A.A", "A"},

	// These should not happen, but make sure
	// we don't get an index panic.
	{MethodDecl, "A", "A"},
	{MethodDecl, "A.", ""},
	{MethodDecl, ".A", "A"},
}

func TestIdentName(t *testing.T) {
	for _, test := range identNameTests {
		id := Ident{Name: test.in, Info: makeTypInfo(test.typ, 0, 0)}
		name := id.name()
		if name != test.out {
			t.Errorf("Ident Name (%+v): (%s)", test, name)
		}
	}
}

func TestRemovePackage(t *testing.T) {
	pakA := Pak{Name: "A", ImportPath: "A"}
	pakB := Pak{Name: "B", ImportPath: "B"}
	exports := map[string]map[string]Ident{
		"A": map[string]Ident{
			"A1":   Ident{Name: "A1", Package: "A", Info: makeTypInfo(ConstDecl, 1, 1)},
			"A2":   Ident{Name: "A2", Package: "A", Info: makeTypInfo(VarDecl, 2, 2)},
			"A3":   Ident{Name: "A3", Package: "A", Info: makeTypInfo(FuncDecl, 3, 3)},
			"A4.M": Ident{Name: "A4.M", Package: "A", Info: makeTypInfo(MethodDecl, 4, 4)},
		},
		"B": map[string]Ident{
			"B1":   Ident{Name: "B1", Package: "B", Info: makeTypInfo(ConstDecl, 1, 1)},
			"B2":   Ident{Name: "B2", Package: "B", Info: makeTypInfo(VarDecl, 2, 2)},
			"B3":   Ident{Name: "B3", Package: "B", Info: makeTypInfo(FuncDecl, 3, 3)},
			"B4.M": Ident{Name: "B4.M", Package: "B", Info: makeTypInfo(MethodDecl, 4, 4)},
		},
	}
	idents := make(map[TypKind]map[string][]Ident)
	for _, m := range exports {
		for _, id := range m {
			k := id.Info.Kind()
			if idents[k] == nil {
				idents[k] = make(map[string][]Ident)
			}
			name := id.name()
			idents[k][name] = append(idents[k][name], id)
		}
	}
	packagePath := map[string]map[string]bool{
		"A": map[string]bool{"A": true},
		"B": map[string]bool{"B": true},
	}
	x := &Indexer{
		strings:     make(map[string]string),
		packagePath: packagePath,
		exports:     make(map[string]map[string]Ident),
		idents:      make(map[TypKind]map[string][]Ident),
	}
	// TODO: Remove this copy if we dont use the original maps
	for pkgName, idents := range exports {
		if x.exports[pkgName] == nil {
			x.exports[pkgName] = make(map[string]Ident)
		}
		for n, id := range idents {
			x.exports[pkgName][n] = id
		}
	}
	for tk, idents := range idents {
		if x.idents[tk] == nil {
			x.idents[tk] = make(map[string][]Ident)
		}
		for n, ids := range idents {
			x.idents[tk][n] = make([]Ident, len(ids))
			copy(x.idents[tk][n], ids)
		}
	}
	x.removePackage(pakA)
	if _, ok := x.exports["A"]; ok {
		t.Fatalf("Indexer: failed to remove Pak: (%+v)", pakA)
	}
	if _, ok := x.exports["B"]; !ok {
		t.Fatalf("Indexer: removed Pak: (%+v)", pakB)
	}
	for _, m := range x.idents {
		for _, ids := range m {
			for _, id := range ids {
				if id.Package == pakA.Name {
					t.Errorf("Indexer: failed to remove ident: (%+v)", id)
				}
			}
		}
	}
	if x.packagePath["A"]["A"] {
		t.Errorf("Indexer: failed to remove packagePath: %s", "A")
	}
	if !x.packagePath["B"]["B"] {
		t.Errorf("Indexer: removed packagePath: %s", "B")
	}
}
