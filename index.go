package pkg

import (
	"go/token"
)

type Indexer struct {
	c    *Corpus
	fset *token.FileSet

	packagePath map[string]map[string]bool
}
