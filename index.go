package pkg

import (
	"bytes"
	"go/ast"
	"go/printer"
	"go/token"
	"sync"
)

/*
const (
	PackageClause SpotKind = iota
	ImportDecl
	ConstDecl
	TypeDecl
	VarDecl
	FuncDecl
	MethodDecl
	Use
	nKinds
)
*/

type TypKind uint64

const (
	InvalidDecl TypKind = iota
	ConstDecl
	VarDecl
	TypeDecl
	FuncDecl
	MethodDecl
	InterfaceDecl

	lastKind
)

var kindNames = [...]string{
	"InvalidDecl",
	"ConstDecl",
	"VarDecl",
	"TypeDecl",
	"FuncDecl",
	"MethodDecl",
	"InterfaceDecl",
}

func (t TypKind) String() string {
	// Make sure lastKind is up to date.
	if t < lastKind {
		return kindNames[t]
	}
	return kindNames[0]
}

func (t TypKind) Name() string { return t.String() }

func convertKind(k ast.ObjKind) TypKind {
	// Warn: What type should interface be?
	switch k {
	case ast.Bad, ast.Pkg:
		return InvalidDecl
	case ast.Con:
		return ConstDecl
	case ast.Typ:
		return TypeDecl
	case ast.Var:
		return VarDecl
	case ast.Fun:
		return FuncDecl
	case ast.Lbl:
		// IDK
	}
	return InvalidDecl
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
func (x TypInfo) IsIndex() bool { return x&1 != 0 }

type Ident struct {
	Name    string  // Type, func or method name
	Package string  // Package name "http"
	Path    string  // Package path "net/http"
	Info    TypInfo // Type and position info
}

func newIdent(id *ast.Ident, recv string, kind TypKind, fset *token.FileSet) *Ident {
	p := positionFor(id.Pos(), fset)
	return &Ident{
		Name: id.Name,
		// Recv: recv,
		Info: makeTypInfo(kind, p.Offset, p.Line),
	}
}

func (i *Ident) IsExported() bool {
	return ast.IsExported(i.Name)
}

type Indexer struct {
	c           *Corpus
	fset        *token.FileSet
	current     *Package
	strings     map[string]string           // interned strings
	packagePath map[string]map[string]bool  // "http" => "net/http" => true
	exports     map[string]map[string]Ident // "net/http" => "Client.Do" => ident
	currExports map[string]Ident
	idents      map[TypKind]map[string][]Ident // Method => "Do" => []ident
}

func (x *Indexer) intern(s string) string {
	if s, ok := x.strings[s]; ok {
		return s
	}
	x.strings[s] = s
	return s
}

func (x *Indexer) addIdent(tk TypKind, ident, recv *ast.Ident) {
	// WARN: Add to 'exports' as well ???
	// WARN: Only add exported idents ???

	if x.currExports == nil {
		x.currExports = make(map[string]Ident)
	}
	if x.idents[tk] == nil {
		x.idents[tk] = make(map[string][]Ident)
	}
	pos := positionFor(ident.Pos(), x.fset)
	name := x.intern(ident.Name)
	id := Ident{
		Name:    name,
		Package: x.intern(x.current.Name),
		Path:    x.intern(x.current.ImportPath),
		Info:    makeTypInfo(tk, pos.Offset, pos.Line),
	}
	// Change the name of methods to be "<typename>.<methodname>".
	// They will still be indexed as <methodname>.
	if tk == MethodDecl && recv != nil {
		id.Name = x.intern(id.Name + "." + recv.Name)
	}

	// Index as <methodname>
	x.idents[tk][name] = append(x.idents[tk][name], id)

	// Index as <typename>.<methodname>
	x.currExports[id.Name] = id
}

func (x *Indexer) Visit(node ast.Node) {
	switch n := node.(type) {
	case *ast.Ident:
	case *ast.ValueSpec:
	case *ast.InterfaceType:
		// x.addIdent(InterfaceDecl, n., nil)
	case *ast.FuncDecl:
		if n.Recv != nil {
			x.visitRecv(n, n.Recv)
		} else {
			x.addIdent(FuncDecl, n.Name, nil)
		}
	default:
		_ = n
	}
}

func (x *Indexer) visitField(field *ast.Field) {

}

func (x *Indexer) visitInterface(ident *ast.Ident, fields *ast.FieldList) {
	for _, field := range fields.List {
		for _, id := range field.Names {
			x.addIdent(InterfaceDecl, ident, id)
		}
	}
}

func (x *Indexer) visitFieldList(ident *ast.Ident, fields *ast.FieldList) {
	for _, field := range fields.List {
		switch n := field.Type.(type) {
		case *ast.Ident:
			x.addIdent(MethodDecl, ident, n)
		case *ast.StarExpr:
			if id, ok := n.X.(*ast.Ident); ok {
				x.addIdent(MethodDecl, ident, id)
			}
		case *ast.FuncType:
			for _, id := range field.Names {
				x.addIdent(InterfaceDecl, ident, id)
			}
		}
	}
}

func (x *Indexer) visitRecv(fn *ast.FuncDecl, fields *ast.FieldList) {
	if len(fields.List) == 0 {
		return
	}
	var recv *ast.Ident
	switch n := fields.List[0].Type.(type) {
	case *ast.StarExpr:
		if id, ok := n.X.(*ast.Ident); ok {
			recv = id
		}
	case *ast.Ident:
		recv = n
	default:
		return
	}
	x.addIdent(MethodDecl, fn.Name, recv)
}

func (x *Indexer) collectIdents(af *ast.File, fset *token.FileSet) []*Ident {
	if af.Decls == nil {
		return nil
	}
	var idents []*Ident
	for _, decl := range af.Decls {
		if !decl.Pos().IsValid() {
			continue
		}
		switch n := decl.(type) {
		case *ast.FuncDecl:
			if i := funcDecl(n, fset); i != nil {
				idents = append(idents, i)
			}
		case *ast.GenDecl:
			idents = genDecl(n, idents, fset)
		}
	}
	return idents
}

func genDecl(n *ast.GenDecl, idents []*Ident, fset *token.FileSet) []*Ident {
	if n == nil || n.Specs == nil {
		return idents
	}
	for _, spec := range n.Specs {
		switch sp := spec.(type) {
		case *ast.TypeSpec:
			if validDecl(sp.Name) {
				idents = append(idents, newIdent(sp.Name, "", TypeDecl, fset))
			}
		case *ast.ValueSpec:
			idents = valueSpec(sp, idents, fset)
		}
	}
	return idents
}

func valueSpec(sp *ast.ValueSpec, idents []*Ident, fset *token.FileSet) []*Ident {
	if sp == nil || sp.Names == nil {
		return idents
	}
	for _, name := range sp.Names {
		if !validDecl(name) {
			continue
		}
		// WARN: Make sure I didnt f'up the ast.Con part
		switch k := name.Obj.Kind; k {
		case ast.Typ, ast.Fun, ast.Var, ast.Con:
			idents = append(idents, newIdent(name, "", convertKind(k), fset))
		}
	}
	return idents
}

func funcDecl(fn *ast.FuncDecl, fset *token.FileSet) *Ident {
	if fn == nil || !validDecl(fn.Name) {
		return nil
	}
	switch {
	case fn.Recv != nil:
		if len(fn.Recv.List) != 0 {
			return newIdent(fn.Name, receiverName(fn, fset), MethodDecl, fset)
		}
	case fn.Name.Name == "init":
		return newIdent(fn.Name, "", FuncDecl, fset)
	}
	return nil
}

func receiverName(fn *ast.FuncDecl, fset *token.FileSet) (recv string) {
	// WARN: Use method from 'define' pkg
	b := getBuffer()
	if printer.Fprint(b, fset, fn.Recv.List[0].Type) == nil {
		recv = b.String()
	}
	putBuffer(b)
	return recv
}

func validDecl(id *ast.Ident) bool {
	return id != nil && id.Pos().IsValid() && id.Name != "_"
}

// positionFor, returns the Position for Pos p without panicking.  If Pos is
// invalid NoPosition is returned.
func positionFor(p token.Pos, fset *token.FileSet) token.Position {
	if p != token.NoPos && fset != nil {
		if f := fset.File(p); f != nil {
			// Prevent panic
			if f.Base() <= int(p) && int(p) <= f.Base()+f.Size() {
				return f.Position(p)
			}
		}
	}
	return token.Position{}
}

var bufferPool sync.Pool

func getBuffer() *bytes.Buffer {
	if v := bufferPool.Get(); v != nil {
		if b, ok := v.(*bytes.Buffer); ok {
			b.Reset()
			return b
		}
	}
	return new(bytes.Buffer)
}

func putBuffer(b *bytes.Buffer) {
	const MB = 1024 * 1024
	// Don't store large buffers
	if b.Cap() <= MB {
		b.Reset()
		bufferPool.Put(b)
	}
}
