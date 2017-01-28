package pkg

import (
	"go/ast"
	"go/parser"
	"go/token"
	pathpkg "path"

	"github.com/charlievieth/pkg/fs"
)

func parseFileName(fset *token.FileSet, filename string) (name string, ok bool) {
	src, err := fs.ReadFile(filename)
	if err != nil {
		return "", false
	}
	af, _ := parser.ParseFile(fset, filename, src, parser.PackageClauseOnly)
	if af != nil && af.Name != nil {
		name = af.Name.Name
	}
	return name, name != ""
}

func parseFile(fset *token.FileSet, filename string, mode parser.Mode) (*ast.File, error) {
	src, err := fs.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	return parser.ParseFile(fset, filename, src, mode)
}

func parseFiles(fset *token.FileSet, dirname string, names []string) (map[string]*ast.File, error) {
	files := make(map[string]*ast.File, len(names))
	for _, n := range names {
		p := pathpkg.Join(dirname, n)
		af, err := parseFile(fset, p, parser.ParseComments)
		if err != nil {
			return nil, err
		}
		files[n] = af
	}
	return files, nil
}
