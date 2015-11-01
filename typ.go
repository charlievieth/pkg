package pkg

import "go/ast"

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
