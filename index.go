package pkg

import (
	"go/ast"
	"go/token"
)

type Ident struct {
	Name    string  // Type, func or method name
	Package string  // Package name "http"
	Path    string  // Package path "net/http"
	Info    TypInfo // Type and position info
}

func (i *Ident) IsExported() bool {
	return ast.IsExported(i.Name)
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
	// TODO (CEV): Add interface methods.
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

func (x *Indexer) index() {
	for _, d := range x.c.dirs {
		x.indexDirectory(d)
	}
}

func (x *Indexer) indexDirectory(d *Directory) {
	if d.Pkg != nil {
		x.indexPackage(d.Pkg)
	}
	for _, d := range d.Dirs {
		x.indexDirectory(d)
	}
}

func (x *Indexer) indexPackage(p *Package) {
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
