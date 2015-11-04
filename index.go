package pkg

import (
	"go/ast"
	"go/token"
	"strings"
)

type Ident struct {
	Name    string  // Type, func or type.method name
	Package string  // Package name "http"
	Path    string  // Package path "net/http"
	File    string  // File where declared "$GOROOT/src/net/http/server.go"
	Info    TypInfo // Type and position info
}

func (i *Ident) IsExported() bool {
	return ast.IsExported(i.Name)
}

// name, returns the name of the ident.  If the ident is a method the typename
// is stripped off, i.e. 'fmt.Print' => 'Print'.
func (i *Ident) name() string {
	if i.Info.Kind() == MethodDecl {
		if n := strings.IndexByte(i.Name, '.'); n != -1 {
			return i.Name[n+1:]
		}
	}
	return i.Name
}

type Indexer struct {
	c       *Corpus
	fset    *token.FileSet
	current *Package

	strings     map[string]string              // interned strings
	packagePath map[string]map[string]bool     // "http" => "net/http" => true
	exports     map[string]map[string]Ident    // "net/http" => "Client.Do" => ident
	idents      map[TypKind]map[string][]Ident // Method => "Do" => []ident

	// TODO: See if we are better off removing 'currExports'
	currExports map[string]Ident // current package
}

func newIndexer(c *Corpus) *Indexer {
	return &Indexer{
		c:           c,
		fset:        token.NewFileSet(),
		strings:     make(map[string]string),
		packagePath: make(map[string]map[string]bool),
		exports:     make(map[string]map[string]Ident),
		idents:      make(map[TypKind]map[string][]Ident),
		currExports: make(map[string]Ident),
	}
}

// WARN: Dev only
func (x *Indexer) PackagePath() map[string]map[string]bool {
	return x.packagePath
}

// WARN: Dev only
func (x *Indexer) Exports() map[string]map[string]Ident {
	return x.exports
}

// WARN: Dev only
func (x *Indexer) Idents() map[TypKind]map[string][]Ident {
	return x.idents
}

func (x *Indexer) intern(s string) string {
	if s, ok := x.strings[s]; ok {
		return s
	}
	x.strings[s] = s
	return s
}

// positionFor, returns the Position for Pos p without panicking.  If Pos is
// invalid NoPosition is returned.
func (x *Indexer) position(p token.Pos) token.Position {
	if p != token.NoPos && x.fset != nil {
		if f := x.fset.File(p); f != nil {
			// Prevent panic
			if f.Base() <= int(p) && int(p) <= f.Base()+f.Size() {
				return f.Position(p)
			}
		}
	}
	return token.Position{}
}

func validIdent(id *ast.Ident) bool {
	return id != nil && id.Name != "_"
}

func (x *Indexer) addIdent(tk TypKind, ident, recv *ast.Ident) {
	// WARN: Add to 'exports' as well ???
	// WARN: Only add exported idents ???
	if !validIdent(ident) {
		return
	}
	if x.currExports == nil {
		x.currExports = make(map[string]Ident)
	}
	if x.idents[tk] == nil {
		x.idents[tk] = make(map[string][]Ident)
	}
	pos := x.position(ident.Pos())
	name := x.intern(ident.Name)
	id := Ident{
		Name:    name,
		Package: x.intern(x.current.Name),
		Path:    x.intern(x.current.ImportPath),
		File:    x.intern(pos.Filename),
		Info:    makeTypInfo(tk, pos.Offset, pos.Line),
	}
	// Change the name of methods to be "<typename>.<methodname>".
	// They will still be indexed as <methodname>.
	if tk == MethodDecl && recv != nil {
		id.Name = x.intern(recv.Name + "." + id.Name)
	}

	// Index as <methodname>
	x.idents[tk][name] = append(x.idents[tk][name], id)

	// Index as <typename>.<methodname>
	x.currExports[id.Name] = id
}

func (x *Indexer) removePackage(p Pak) {
	if x.exports == nil {
		return
	}
	exp := x.exports[p.ImportPath]
	if exp == nil {
		return
	}
	idents := make(map[TypKind]map[string]map[Ident]bool)
	for _, id := range exp {
		k := id.Info.Kind()
		if idents[k] == nil {
			idents[k] = make(map[string]map[Ident]bool)
		}
		name := id.name()
		if idents[k][name] == nil {
			idents[k][name] = make(map[Ident]bool)
		}
		idents[k][name][id] = true
	}
	filter := func(m map[Ident]bool, ids []Ident) []Ident {
		n := 0
		for i := 0; i < len(ids); i++ {
			if !m[ids[i]] {
				ids[n] = ids[i]
				n++
			}
		}
		return ids[:n]
	}
	for kind, names := range idents {
		for name, ids := range names {
			xids := filter(ids, x.idents[kind][name])
			if len(xids) > 0 {
				x.idents[kind][name] = xids
			} else {
				delete(x.idents[kind], name)
			}
		}
	}
	delete(x.packagePath[p.Name], p.ImportPath)
	delete(x.exports, p.ImportPath)
}

func (x *Indexer) visitRecv(fn *ast.FuncDecl, fields *ast.FieldList) {
	if len(fields.List) != 0 {
		switch n := fields.List[0].Type.(type) {
		case *ast.Ident:
			x.addIdent(MethodDecl, fn.Name, n)
		case *ast.StarExpr:
			if id, ok := n.X.(*ast.Ident); ok {
				x.addIdent(MethodDecl, fn.Name, id)
			}
		}
	}
}

func (x *Indexer) visitGenDecl(decl *ast.GenDecl) {
	for _, spec := range decl.Specs {
		switch n := spec.(type) {
		case *ast.TypeSpec:
			x.addIdent(TypeDecl, n.Name, nil)
		case *ast.ValueSpec:
			x.visitValueSpec(n)
		}
	}
}

func (x *Indexer) visitValueSpec(spec *ast.ValueSpec) {
	// TODO (CEV): Add interface methods.
	for _, n := range spec.Names {
		if n.Obj == nil {
			continue
		}
		switch n.Obj.Kind {
		case ast.Con:
			x.addIdent(ConstDecl, n, nil)
		case ast.Typ:
			x.addIdent(TypeDecl, n, nil)
		case ast.Var:
			x.addIdent(VarDecl, n, nil)
		case ast.Fun:
			x.addIdent(FuncDecl, n, nil)
		}
	}
}

func (x *Indexer) visitFile(af *ast.File) {
	for _, d := range af.Decls {
		switch n := d.(type) {
		case *ast.FuncDecl:
			if n.Recv != nil {
				x.visitRecv(n, n.Recv)
			} else {
				// WARN: We may be adding the file twice!!!
				x.addIdent(FuncDecl, n.Name, nil)
			}
		case *ast.GenDecl:
			x.visitGenDecl(n)
		}
	}
}

// Visit, walks ast Files and Packages only - use visitFile instead.
func (x *Indexer) Visit(node ast.Node) ast.Visitor {
	switch n := node.(type) {
	case *ast.File:
		x.visitFile(n)
	case *ast.Package:
		for _, f := range n.Files {
			ast.Walk(x, f)
		}
	}
	return nil
}

func (x *Indexer) indexDirectory(d *Directory) {
	if !x.c.IndexEnabled {
		return
	}
	if d.Pkg != nil && !d.Pkg.IsCommand() {
		x.indexPackage(d.Pkg)
	}
	for _, d := range d.Dirs {
		x.indexDirectory(d)
	}
}

func (x *Indexer) indexPackage(p *Package) {
	if p.IsCommand() || !p.IsValid() || !x.c.IndexEnabled {
		return
	}
	files, err := parseFiles(x.fset, p.Dir, p.SrcFiles())
	if err != nil || len(files) == 0 {
		return
	}
	x.current = p
	for _, af := range files {
		x.visitFile(af)
	}
	if x.packagePath == nil {
		x.packagePath = make(map[string]map[string]bool)
	}
	if x.packagePath[p.Name] == nil {
		x.packagePath[p.Name] = make(map[string]bool)
	}
	x.packagePath[p.Name][p.ImportPath] = true
	if x.exports == nil {
		x.exports = make(map[string]map[string]Ident)
	}
	// TODO: See if we are better off removing 'currExports'
	m := make(map[string]Ident, len(x.currExports))
	for k, v := range x.currExports {
		m[k] = v
		delete(x.currExports, k)
	}
	x.exports[p.ImportPath] = m
}
