package pkg

import (
	"errors"
	"go/ast"
	"go/token"
	"os"
	"path/filepath"
	"sync"
)

type stringInterner struct {
	sync.RWMutex
	strings map[string]string
}

func (x *stringInterner) get(s string) (string, bool) {
	x.RLock()
	s, ok := x.strings[s]
	x.RUnlock()
	return s, ok
}

func (x *stringInterner) add(s string) string {
	x.Lock()
	x.strings[s] = s
	x.Unlock()
	return s
}

func (x *stringInterner) intern(s string) string {
	if s, ok := x.get(s); ok {
		return s
	}
	return x.add(s)
}

type Index struct {
	c    *Corpus
	fset *token.FileSet

	strings stringInterner // interned strings

	// AST
	packagePath map[string]map[string]bool     // "http" => "net/http" => true
	exports     map[string]map[string]Ident    // "net/http" => "Client.Do" => ident
	idents      map[TypKind]map[string][]Ident // Method => "Do" => []ident

	// Go Packages
	packages    map[string]map[string]Package
	packageChan chan *packageIndexer
}

func (x *Index) intern(s string) string {
	return x.strings.intern(s)
}

type packageIndexer struct {
	x        *Index
	fset     *token.FileSet
	dir      string
	name     string
	path     string
	dirnames []string
	current  *Package
}

func (x *packageIndexer) indexPackage() {
	fi, err := os.Stat(x.path)
	if err != nil {
		return
	}
	_ = fi
}

func (x *packageIndexer) updatePackage(fi os.FileInfo) {
	if x.current == nil {
		return
	}
	x.current.Info, fi = fi, x.current.Info
	same := sameFile(x.current.Info, fi)
	if err := x.initDirnames(same); err != nil {
		return
	}

}

func (x *packageIndexer) visitFile(name string) (err error) {
	file, found := x.current.LookupFile(name)
	if !found {
		path := filepath.Join(x.dir, name)
		file, err = NewFile(path, true)
		if err != nil {
			return
		}
	} else {
		fi, err := os.Stat(file.Path)
		if err != nil {
			return err
		}
		if fi.IsDir() {
			return errors.New("pkg: invalid file path")
		}
		file.Info, fi = fi, file.Info
		if sameFile(file.Info, fi) {
			return nil
		}
	}
	return nil
}

func (x *packageIndexer) initDirnames(usePkg bool) error {
	if x.dirnames != nil {
		return nil
	}
	if usePkg && x.current.isPkgDir() {
		x.dirnames = x.current.fileNames()
		return nil
	}
	names, err := readdirnames(x.path)
	if err != nil {
		return err
	}
	x.dirnames = names
	return nil
}

func (x *packageIndexer) intern(s string) string {
	return x.x.intern(s)
}

type astIndexer struct {
	x       *Index
	fset    *token.FileSet
	current *Package
	idents  map[TypKind]map[string][]Ident
	exports map[string]Ident
}

func (x *astIndexer) intern(s string) string {
	return x.x.intern(s)
}

func (x *astIndexer) position(p token.Pos) token.Position {
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

func (x *astIndexer) visitIdent(tk TypKind, ident, recv *ast.Ident) {
	if !validIdent(ident) {
		return
	}
	if x.idents[tk] == nil {
		x.idents[tk] = make(map[string][]Ident)
	}
	if x.exports == nil {
		x.exports = make(map[string]Ident)
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
	x.exports[id.Name] = id
}

func (x *astIndexer) visitRecv(fn *ast.FuncDecl, fields *ast.FieldList) {
	if len(fields.List) != 0 {
		switch n := fields.List[0].Type.(type) {
		case *ast.Ident:
			x.visitIdent(MethodDecl, fn.Name, n)
		case *ast.StarExpr:
			if id, ok := n.X.(*ast.Ident); ok {
				x.visitIdent(MethodDecl, fn.Name, id)
			}
		}
	}
}

func (x *astIndexer) visitGenDecl(decl *ast.GenDecl) {
	for _, spec := range decl.Specs {
		switch n := spec.(type) {
		case *ast.TypeSpec:
			x.visitIdent(TypeDecl, n.Name, nil)
		case *ast.ValueSpec:
			x.visitValueSpec(n)
		}
	}
}

func (x *astIndexer) visitValueSpec(spec *ast.ValueSpec) {
	// TODO (CEV): Add interface methods.
	for _, n := range spec.Names {
		if n.Obj == nil {
			continue
		}
		switch n.Obj.Kind {
		case ast.Con:
			x.visitIdent(ConstDecl, n, nil)
		case ast.Typ:
			x.visitIdent(TypeDecl, n, nil)
		case ast.Var:
			x.visitIdent(VarDecl, n, nil)
		case ast.Fun:
			x.visitIdent(FuncDecl, n, nil)
		}
	}
}

func (x *astIndexer) visitFile(af *ast.File) {
	for _, d := range af.Decls {
		switch n := d.(type) {
		case *ast.FuncDecl:
			if n.Recv != nil {
				x.visitRecv(n, n.Recv)
			} else {
				// WARN: We may be adding the file twice!!!
				x.visitIdent(FuncDecl, n.Name, nil)
			}
		case *ast.GenDecl:
			x.visitGenDecl(n)
		}
	}
}

// Visit, walks ast Files and Packages only - use visitFile instead.
func (x *astIndexer) Visit(node ast.Node) ast.Visitor {
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
