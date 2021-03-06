package pkg

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
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

func TestMergeIdents(t *testing.T) {
	// TODO: organize and add more test cases

	exports := map[string]map[string]Ident{
		"A": map[string]Ident{
			"A1":   Ident{Name: "A1", Package: "A", Info: makeTypInfo(ConstDecl, 1, 1)},
			"A2":   Ident{Name: "A2", Package: "A", Info: makeTypInfo(VarDecl, 2, 2)},
			"A3":   Ident{Name: "A3", Package: "A", Info: makeTypInfo(FuncDecl, 3, 3)},
			"A4.M": Ident{Name: "A4.M", Package: "A", Info: makeTypInfo(MethodDecl, 4, 4)},
		},
	}
	expA := make(map[string]Ident)
	for name, id := range exports["A"] {
		expA[name] = id
	}
	added := map[Ident]bool{
		Ident{Name: "A5.M", Package: "A", Info: makeTypInfo(MethodDecl, 4, 4)}: true,
		Ident{Name: "A6", Package: "A", Info: makeTypInfo(FuncDecl, 6, 6)}:     true,
	}
	removed := map[Ident]bool{
		Ident{Name: "A3", Package: "A", Info: makeTypInfo(FuncDecl, 3, 3)}:     true,
		Ident{Name: "A4.M", Package: "A", Info: makeTypInfo(MethodDecl, 4, 4)}: true,
	}
	expB := make(map[string]Ident)
	for name, id := range exports["A"] {
		if !removed[id] {
			expB[name] = id
		}
	}
	for id := range added {
		expB[id.Name] = id
	}
	identsB := make(map[TypKind]map[string][]Ident)
	for _, id := range expB {
		k := id.Info.Kind()
		if identsB[k] == nil {
			identsB[k] = make(map[string][]Ident)
		}
		name := id.name()
		identsB[k][name] = append(identsB[k][name], id)
	}
	for _, m := range identsB {
		for _, ids := range m {
			for _, id := range ids {
				if removed[id] {
					t.Fatalf("Merge: internal error added (%+v)", id)
				}
			}
		}
	}
	identsA := make(map[TypKind]map[string][]Ident)
	for _, m := range exports {
		for _, id := range m {
			k := id.Info.Kind()
			if identsA[k] == nil {
				identsA[k] = make(map[string][]Ident)
			}
			name := id.name()
			identsA[k][name] = append(identsA[k][name], id)
		}
	}
	packagePath := map[string]map[string]bool{
		"A": map[string]bool{"A": true},
	}
	x := &Index{
		packagePath: packagePath,
		exports:     exports,
		idents:      identsA,
	}
	x.mergeIdents(expA, expB)
	seen := make(map[Ident]bool)
	for _, m := range x.idents {
		for _, ids := range m {
			for _, id := range ids {
				seen[id] = true
				if removed[id] {
					t.Errorf("Merge: did not remove (%+v)", id)
				}
			}
		}
	}
	for _, m := range identsB {
		for _, ids := range m {
			for _, id := range ids {
				if !seen[id] {
					t.Errorf("Merge: did not add (%+v)", id)
				}
			}
		}
	}
}

func TestRemovePackage(t *testing.T) {
	// TODO: organize and add more test cases

	pakA := &Package{Name: "A", ImportPath: "A"}
	pakB := &Package{Name: "B", ImportPath: "B"}
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
	x := &Index{
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
		t.Fatalf("Index: failed to remove Pak: (%+v)", pakA)
	}
	if _, ok := x.exports["B"]; !ok {
		t.Fatalf("Index: removed Pak: (%+v)", pakB)
	}
	for _, m := range x.idents {
		for _, ids := range m {
			for _, id := range ids {
				if id.Package == pakA.Name {
					t.Errorf("Index: failed to remove ident: (%+v)", id)
				}
			}
		}
	}
	if x.packagePath["A"]["A"] {
		t.Errorf("Index: failed to remove packagePath: %s", "A")
	}
	if !x.packagePath["B"]["B"] {
		t.Errorf("Index: removed packagePath: %s", "B")
	}
}

func BenchmarkAstIndexer(b *testing.B) {
	filename := filepath.Join(runtime.GOROOT(), "src/crypto/x509/x509.go")
	if _, err := os.Stat(filename); err != nil {
		b.Skipf("cannot stat (%s): %s", filename, err)
	}
	fset := token.NewFileSet()
	pkg := &Package{
		Dir: filepath.Dir(filename),
		files: map[GoFileType]FileMap{
			GoFile: map[string]File{
				filepath.Base(filename): File{Name: filename},
			},
		},
	}
	af, err := parser.ParseFile(fset, filename, nil, 0)
	if err != nil {
		b.Fatal(err)
	}
	ax := &astIndexer{
		x:       newIndex(nil),
		fset:    fset,
		current: pkg,
		exports: make(map[string]Ident),
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ax.Visit(af)
		b.StopTimer()
		ax.exports = make(map[string]Ident)
		b.StartTimer()
	}
}
