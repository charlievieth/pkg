package pkg2

import (
	"fmt"
	"git.vieth.io/pkg2/fs"
	"go/ast"
	"go/token"
	"strings"
	"sync"

	"git.vieth.io/pkg2/util"
)

type Ident struct {
	Name    string  // Type, func or type.method name
	Package string  // Package name "http"
	Path    string  // Package path "net/http"
	File    string  // File where declared "$GOROOT/src/net/http/server.go"
	Info    TypInfo // Type and position info
}

// name, returns the name of the ident.  If the ident is a method the typename
// is stripped off, i.e. 'fmt.Print' => 'Print'.
func (i *Ident) name() string {
	switch i.Info.Kind() {
	case MethodDecl, InterfaceDecl:
		if n := strings.IndexByte(i.Name, '.'); n != -1 {
			return i.Name[n+1:]
		}
	}
	return i.Name
}

// IsExported reports whether the Ident is an exported Go symbol.
func (i *Ident) IsExported() bool { return ast.IsExported(i.Name) }

type IndexEvent struct {
	typ EventType
	msg string
}

func (e IndexEvent) Event() EventType         { return e.typ }
func (e IndexEvent) Callback(c *Corpus) error { return nil }
func (e IndexEvent) String() string           { return e.msg }

type Index struct {
	c           *Corpus
	fset        *token.FileSet
	strings     util.StringInterner            // interned strings
	packagePath map[string]map[string]bool     // "http" => "net/http" => true
	exports     map[string]map[string]Ident    // "net/http" => "Client.Do" => ident
	idents      map[TypKind]map[string][]Ident // Method => "Do" => []ident
	mu          sync.RWMutex
}

func newIndex(c *Corpus) *Index {
	return &Index{
		c:           c,
		fset:        token.NewFileSet(),
		packagePath: make(map[string]map[string]bool),
		exports:     make(map[string]map[string]Ident),
		idents:      make(map[TypKind]map[string][]Ident),
	}
}

func (x *Index) notify(typ EventType, path string) {
	if x.c == nil || !x.c.LogEvents {
		return
	}
	e := IndexEvent{
		typ: typ,
		msg: fmt.Sprintf("Index: %s %q", typ, path),
	}
	x.c.notify(e)
}

func (x *Index) errorEvent(err error, path string) {
	if x.c == nil {
		return
	}
	e := IndexEvent{
		typ: DeleteEvent,
		msg: fmt.Sprintf(`Index: error updating package "%s": %s`, path, err),
	}
	x.c.notify(e)
}

func (x *Index) intern(s string) string {
	return x.strings.Intern(s)
}

func (x *Index) lookupExports(importPath string) map[string]Ident {
	if x.exports == nil {
		return nil
	}
	x.mu.RLock()
	exp := x.exports[importPath]
	x.mu.RUnlock()
	return exp
}

func (x *Index) hasPackage(importPath string) bool {
	x.mu.RLock()
	_, ok := x.exports[importPath]
	x.mu.RUnlock()
	return ok
}

// initMaps, inits the Index's maps.  Lock the mutex for writing before calling.
func (x *Index) initMaps() {
	if x.exports == nil {
		x.exports = make(map[string]map[string]Ident)
	}
	if x.packagePath == nil {
		x.packagePath = make(map[string]map[string]bool)
	}
	if x.idents == nil {
		x.idents = make(map[TypKind]map[string][]Ident)
	}
}

// removePackageIdents, removes the idents of the Package with name name and
// import path path.  The Package's exports must still be indexed.
//
// Lock the Index's mutex for writing before calling.
func (x *Index) removePackage(p *Package) {
	if !x.hasPackage(p.ImportPath) {
		return
	}
	x.mu.Lock()
	// Send event after releasing mutex.
	defer x.notify(DeleteEvent, p.ImportPath)
	defer x.mu.Unlock()

	// Returns ids with any Ident found in m removed.
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

	// Use exports to map the idents we need to remove.
	// TODO: Improve - see the merge method for reference.
	idents := make(map[TypKind]map[string]map[Ident]bool)
	for _, id := range x.exports[p.ImportPath] {
		tk := id.Info.Kind()
		if idents[tk] == nil {
			idents[tk] = make(map[string]map[Ident]bool)
		}
		name := id.name()
		if idents[tk][name] == nil {
			idents[tk][name] = make(map[Ident]bool)
		}
		idents[tk][name][id] = true
	}

	// Remove idents.
	for kind, names := range idents {
		for name, ids := range names {
			xids := filter(ids, x.idents[kind][name])
			if len(xids) > 0 {
				x.idents[kind][name] = xids
			} else {
				delete(x.idents[kind], name)
				if len(x.idents[kind]) == 0 {
					delete(x.idents, kind)
				}
			}
		}
	}

	delete(x.packagePath[p.Name], p.ImportPath)
	delete(x.exports, p.ImportPath)
}

// mergeIdents, removes the Idents from oldExp not present in newExp, and adds
// the Idents in newExp not present in oldExp.
//
// Lock the Index's mutex for writing before calling.
func (x *Index) mergeIdents(oldExp, newExp map[string]Ident) {

	// Removes id from slice ids.
	filter := func(id Ident, ids []Ident) []Ident {
		n := 0
		for i := 0; i < len(ids); i++ {
			if ids[i] != id {
				ids[n] = ids[i]
				n++
			}
		}
		return ids[:n]
	}

	del := make(map[Ident]bool)
	add := make(map[Ident]bool)
	for _, id := range oldExp {
		del[id] = true
	}
	for _, id := range newExp {
		if del[id] {
			delete(del, id)
		} else {
			add[id] = true
		}
	}
	for id := range del {
		tk := id.Info.Kind()
		name := id.name()
		xids := filter(id, x.idents[tk][name])
		if len(xids) > 0 {
			x.idents[tk][name] = xids
		} else {
			delete(x.idents[tk], name)
			if len(x.idents[tk]) == 0 {
				delete(x.idents, tk)
			}
		}
	}
	for id := range add {
		tk := id.Info.Kind()
		name := id.name()
		if x.idents[tk] == nil {
			x.idents[tk] = make(map[string][]Ident)
		}
		x.idents[tk][name] = append(x.idents[tk][name], id)
	}
}

// mergeAST, merges the Idents from ax into the index, removing any Idents
// no longer present in the package.
func (x *Index) mergeAST(ax *astIndexer) {
	x.mu.Lock()
	defer x.mu.Unlock()
	x.initMaps()
	x.mergeIdents(x.exports[ax.current.Name], ax.exports)
	x.exports[ax.current.Name] = ax.exports
}

// addAST, adds the Idents from ax to the index.
func (x *Index) addAST(ax *astIndexer) {
	// Double check that the package does not exist,
	// Otherwise we will end up with duplicate idents.
	if x.hasPackage(ax.current.ImportPath) {
		x.mergeAST(ax)
		return
	}
	x.mu.Lock()
	defer x.mu.Unlock()

	x.initMaps()
	x.exports[ax.current.Name] = ax.exports
	if x.packagePath[ax.current.Name] == nil {
		x.packagePath[ax.current.Name] = make(map[string]bool)
	}
	x.packagePath[ax.current.Name][ax.current.ImportPath] = true
	for tk, m := range ax.idents {
		if x.idents[tk] == nil {
			x.idents[tk] = make(map[string][]Ident)
		}
		idents := x.idents[tk]
		for n, ids := range m {
			idents[n] = append(idents[n], ids...)
		}
	}
}

// indexPackage, indexes Package p.  If the Package is already indexed, any
// changes will be merged in.
func (x *Index) indexPackage(p *Package) {
	if !x.c.IndexEnabled || p.IsCommand() || !p.IsValid() {
		return
	}
	ax := &astIndexer{
		x:       x,
		fset:    x.fset,
		current: p,
		exports: make(map[string]Ident),
	}
	// Only init the idents map if we are adding a new
	// package, it is not used for merging updates.
	update := x.hasPackage(p.ImportPath)
	if !update {
		ax.idents = make(map[TypKind]map[string][]Ident)
	}
	// The error is either a os.PathError or parser error.
	// For now, ignore AST errors, but delete on PathError.
	if err := ax.index(); err != nil {
		x.errorEvent(err, p.ImportPath)
		if update && fs.IsPathErr(err) {
			x.removePackage(p)
		}
		return
	}
	if update {
		x.mergeAST(ax)
		x.notify(UpdateEvent, p.ImportPath)
	} else {
		x.addAST(ax)
		x.notify(CreateEvent, p.ImportPath)
	}
}

// WARN: NEW
func (x *Index) indexPackageFiles(p *Package, fset *token.FileSet, files map[string]*ast.File) {
	if !x.c.IndexEnabled || p.IsCommand() || !p.IsValid() {
		return
	}
	if len(files) == 0 {
		x.indexPackage(p)
		return
	}
	ax := &astIndexer{
		x:       x,
		fset:    fset,
		current: p,
		exports: make(map[string]Ident),
	}
	// Only init the idents map if we are adding a new
	// package, it is not used for merging updates.
	update := x.hasPackage(p.ImportPath)
	if !update {
		ax.idents = make(map[TypKind]map[string][]Ident)
	}
	ax.indexFiles(files)
	if update {
		x.mergeAST(ax)
		x.notify(UpdateEvent, p.ImportPath)
	} else {
		x.addAST(ax)
		x.notify(CreateEvent, p.ImportPath)
	}
}

type astIndexer struct {
	x       *Index
	fset    *token.FileSet
	current *Package
	exports map[string]Ident
	idents  map[TypKind]map[string][]Ident // Only updated if not nill.
}

func (x *astIndexer) index() error {
	files, err := parseFiles(x.fset, x.current.Dir, x.current.GoFiles())
	if err != nil {
		return err
	}
	return x.indexFiles(files)
}

func (x *astIndexer) indexFiles(files map[string]*ast.File) error {
	for _, af := range files {
		x.Visit(af)
	}
	return nil
}

func (x *astIndexer) intern(s string) string {
	return x.x.strings.Intern(s)
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

func validIdent(id *ast.Ident) bool {
	return id != nil && id.Name != "_"
}

func (x *astIndexer) visitIdent(tk TypKind, ident, recv *ast.Ident) {
	if !validIdent(ident) {
		return
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

	// If nil, don't update.
	if x.idents != nil {
		if x.idents[tk] == nil {
			x.idents[tk] = make(map[string][]Ident)
		}
		// Index as <methodname>
		x.idents[tk][name] = append(x.idents[tk][name], id)
	}

	if x.exports == nil {
		x.exports = make(map[string]Ident)
	}
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
