package pkg

import (
	"bytes"
	"go/ast"
	"go/printer"
	"go/token"
	"sync"
	"unicode"
	"unicode/utf8"
)

type TypKind uint64

const (
	Invalid TypKind = iota
	Const
	Var
	TypeName
	Func
	Method
	Interface

	lastKind
)

var kindNames = [...]string{
	"Invalid",
	"Const",
	"Var",
	"TypeName",
	"Func",
	"Method",
	"Interface",
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
		return Invalid
	case ast.Con:
		return Const
	case ast.Typ:
		return TypeName
	case ast.Var:
		return Var
	case ast.Fun:
		return Func
	case ast.Lbl:
		// IDK
	}
	return Invalid
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
	Name string  // Type, func or method name
	Recv string  // Receiver if method
	Info TypInfo // Type and position info
}

func (i *Ident) IsExported() bool {
	ch, _ := utf8.DecodeRuneInString(i.Name)
	return unicode.IsUpper(ch)
}

func newIdent(id *ast.Ident, recv string, kind TypKind, fset *token.FileSet) *Ident {
	p := positionFor(id.Pos(), fset)
	return &Ident{
		Name: id.Name,
		Recv: recv,
		Info: makeTypInfo(kind, p.Offset, p.Line),
	}
}

type Indexer struct {
	c    *Corpus
	fset *token.FileSet
	token.Position
	packagePath   map[string]map[string]bool
	packageIdents map[string]map[string]*Ident
	idents        map[TypKind]map[string][]*Ident
}

func (i *Indexer) collectIdents(af *ast.File, fset *token.FileSet) []*Ident {
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
				idents = append(idents, newIdent(sp.Name, "", TypeName, fset))
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
			return newIdent(fn.Name, receiverName(fn, fset), Method, fset)
		}
	case fn.Name.Name == "init":
		return newIdent(fn.Name, "", Func, fset)
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
