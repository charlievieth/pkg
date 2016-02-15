// Copyright 2011 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build go1.5

// Backported vendor support from Go 1.6 to Go 1.5.  Only Go 1.5 needs to
// backported support.

package buildutil

import (
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/build"
	"go/doc"
	"go/parser"
	"go/token"
	"io"
	"io/ioutil"
	"log"
	"os"
	pathpkg "path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

type context struct {
	build.Context
}

func newContext(c *build.Context) *context {
	if c != nil {
		return &context{*c}
	}
	return &context{build.Default}
}

// joinPath calls ctxt.JoinPath (if not nil) or else filepath.Join.
func (ctxt *context) joinPath(elem ...string) string {
	if f := ctxt.JoinPath; f != nil {
		return f(elem...)
	}
	return filepath.Join(elem...)
}

// splitPathList calls ctxt.SplitPathList (if not nil) or else filepath.SplitList.
func (ctxt *context) splitPathList(s string) []string {
	if f := ctxt.SplitPathList; f != nil {
		return f(s)
	}
	return filepath.SplitList(s)
}

// isAbsPath calls ctxt.IsAbsPath (if not nil) or else filepath.IsAbs.
func (ctxt *context) isAbsPath(path string) bool {
	if f := ctxt.IsAbsPath; f != nil {
		return f(path)
	}
	return filepath.IsAbs(path)
}

// isDir calls ctxt.IsDir (if not nil) or else uses os.Stat.
func (ctxt *context) isDir(path string) bool {
	if f := ctxt.IsDir; f != nil {
		return f(path)
	}
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}

// hasSubdir calls ctxt.HasSubdir (if not nil) or else uses
// the local file system to answer the question.
func (ctxt *context) hasSubdir(root, dir string) (rel string, ok bool) {
	if f := ctxt.HasSubdir; f != nil {
		return f(root, dir)
	}

	// Try using paths we received.
	if rel, ok = hasSubdir(root, dir); ok {
		return
	}

	// Try expanding symlinks and comparing
	// expanded against unexpanded and
	// expanded against expanded.
	rootSym, _ := filepath.EvalSymlinks(root)
	dirSym, _ := filepath.EvalSymlinks(dir)

	if rel, ok = hasSubdir(rootSym, dir); ok {
		return
	}
	if rel, ok = hasSubdir(root, dirSym); ok {
		return
	}
	return hasSubdir(rootSym, dirSym)
}

func hasSubdir(root, dir string) (rel string, ok bool) {
	const sep = string(filepath.Separator)
	root = filepath.Clean(root)
	if !strings.HasSuffix(root, sep) {
		root += sep
	}
	dir = filepath.Clean(dir)
	if !strings.HasPrefix(dir, root) {
		return "", false
	}
	return filepath.ToSlash(dir[len(root):]), true
}

// readDir calls ctxt.ReadDir (if not nil) or else ioutil.ReadDir.
func (ctxt *context) readDir(path string) ([]os.FileInfo, error) {
	if f := ctxt.ReadDir; f != nil {
		return f(path)
	}
	return ioutil.ReadDir(path)
}

// openFile calls ctxt.OpenFile (if not nil) or else os.Open.
func (ctxt *context) openFile(path string) (io.ReadCloser, error) {
	if fn := ctxt.OpenFile; fn != nil {
		return fn(path)
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, err // nil interface
	}
	return f, nil
}

// isFile determines whether path is a file by trying to open it.
// It reuses openFile instead of adding another function to the
// list in Context.
func (ctxt *context) isFile(path string) bool {
	f, err := ctxt.openFile(path)
	if err != nil {
		return false
	}
	f.Close()
	return true
}

// gopath returns the list of Go path directories.
func (ctxt *context) gopath() []string {
	var all []string
	for _, p := range ctxt.splitPathList(ctxt.GOPATH) {
		if p == "" || p == ctxt.GOROOT {
			// Empty paths are uninteresting.
			// If the path is the GOROOT, ignore it.
			// People sometimes set GOPATH=$GOROOT.
			// Do not get confused by this common mistake.
			continue
		}
		if strings.HasPrefix(p, "~") {
			// Path segments starting with ~ on Unix are almost always
			// users who have incorrectly quoted ~ while setting GOPATH,
			// preventing it from expanding to $HOME.
			// The situation is made more confusing by the fact that
			// bash allows quoted ~ in $PATH (most shells do not).
			// Do not get confused by this, and do not try to use the path.
			// It does not exist, and printing errors about it confuses
			// those users even more, because they think "sure ~ exists!".
			// The go command diagnoses this situation and prints a
			// useful error.
			// On Windows, ~ is used in short names, such as c:\progra~1
			// for c:\program files.
			continue
		}
		all = append(all, p)
	}
	return all
}

// ImportDir is like Import but processes the Go package found in
// the named directory.
func ImportDir(bc *build.Context, dir string, mode build.ImportMode) (*build.Package, error) {
	return Import(bc, ".", dir, mode)
}

func nameExt(name string) string {
	i := strings.LastIndex(name, ".")
	if i < 0 {
		return ""
	}
	return name[i:]
}

// Import returns details about the Go package named by the import path,
// interpreting local import paths relative to the srcDir directory.
// If the path is a local import path naming a package that can be imported
// using a standard import path, the returned package will set p.ImportPath
// to that path.
//
// In the directory containing the package, .go, .c, .h, and .s files are
// considered part of the package except for:
//
//	- .go files in package documentation
//	- files starting with _ or . (likely editor temporary files)
//	- files with build constraints not satisfied by the context
//
// If an error occurs, Import returns a non-nil error and a non-nil
// *Package containing partial information.
//
func Import(bc *build.Context, path string, srcDir string, mode build.ImportMode) (*build.Package, error) {
	// Convert build.Context to Context.
	ctxt := newContext(bc)

	p := &build.Package{
		ImportPath: path,
	}
	if path == "" {
		return p, fmt.Errorf("import %q: invalid import path", path)
	}

	var pkgtargetroot string
	var pkga string
	var pkgerr error
	suffix := ""
	if ctxt.InstallSuffix != "" {
		suffix = "_" + ctxt.InstallSuffix
	}
	switch ctxt.Compiler {
	case "gccgo":
		pkgtargetroot = "pkg/gccgo_" + ctxt.GOOS + "_" + ctxt.GOARCH + suffix
	case "gc":
		pkgtargetroot = "pkg/" + ctxt.GOOS + "_" + ctxt.GOARCH + suffix
	default:
		// Save error for end of function.
		pkgerr = fmt.Errorf("import %q: unknown compiler %q", path, ctxt.Compiler)
	}
	setPkga := func() {
		switch ctxt.Compiler {
		case "gccgo":
			dir, elem := pathpkg.Split(p.ImportPath)
			pkga = pkgtargetroot + "/" + dir + "lib" + elem + ".a"
		case "gc":
			pkga = pkgtargetroot + "/" + p.ImportPath + ".a"
		}
	}
	setPkga()

	binaryOnly := false
	if build.IsLocalImport(path) {
		pkga = "" // local imports have no installed path
		if srcDir == "" {
			return p, fmt.Errorf("import %q: import relative to unknown directory", path)
		}
		if !ctxt.isAbsPath(path) {
			p.Dir = ctxt.joinPath(srcDir, path)
		}
		// Determine canonical import path, if any.
		// Exclude results where the import path would include /testdata/.
		inTestdata := func(sub string) bool {
			return strings.Contains(sub, "/testdata/") || strings.HasSuffix(sub, "/testdata") || strings.HasPrefix(sub, "testdata/") || sub == "testdata"
		}
		if ctxt.GOROOT != "" {
			root := ctxt.joinPath(ctxt.GOROOT, "src")
			if sub, ok := ctxt.hasSubdir(root, p.Dir); ok && !inTestdata(sub) {
				p.Goroot = true
				p.ImportPath = sub
				p.Root = ctxt.GOROOT
				goto Found
			}
		}
		all := ctxt.gopath()
		for i, root := range all {
			rootsrc := ctxt.joinPath(root, "src")
			if sub, ok := ctxt.hasSubdir(rootsrc, p.Dir); ok && !inTestdata(sub) {
				// We found a potential import path for dir,
				// but check that using it wouldn't find something
				// else first.
				if ctxt.GOROOT != "" {
					if dir := ctxt.joinPath(ctxt.GOROOT, "src", sub); ctxt.isDir(dir) {
						p.ConflictDir = dir
						goto Found
					}
				}
				for _, earlyRoot := range all[:i] {
					if dir := ctxt.joinPath(earlyRoot, "src", sub); ctxt.isDir(dir) {
						p.ConflictDir = dir
						goto Found
					}
				}

				// sub would not name some other directory instead of this one.
				// Record it.
				p.ImportPath = sub
				p.Root = root
				goto Found
			}
		}
		// It's okay that we didn't find a root containing dir.
		// Keep going with the information we have.
	} else {
		if strings.HasPrefix(path, "/") {
			return p, fmt.Errorf("import %q: cannot import absolute path", path)
		}

		// tried records the location of unsuccessful package lookups
		var tried struct {
			vendor []string
			goroot string
			gopath []string
		}
		gopath := ctxt.gopath()

		// Vendor directories get first chance to satisfy import.
		if mode&IgnoreVendor == 0 && srcDir != "" {
			searchVendor := func(root string, isGoroot bool) bool {
				sub, ok := ctxt.hasSubdir(root, srcDir)
				if !ok || !strings.HasPrefix(sub, "src/") || strings.Contains(sub, "/testdata/") {
					return false
				}
				for {
					vendor := ctxt.joinPath(root, sub, "vendor")
					if ctxt.isDir(vendor) {
						dir := ctxt.joinPath(vendor, path)
						if ctxt.isDir(dir) && hasGoFiles(ctxt, dir) {
							p.Dir = dir
							p.ImportPath = strings.TrimPrefix(pathpkg.Join(sub, "vendor", path), "src/")
							p.Goroot = isGoroot
							p.Root = root
							setPkga() // p.ImportPath changed
							return true
						}
						tried.vendor = append(tried.vendor, dir)
					}
					i := strings.LastIndex(sub, "/")
					if i < 0 {
						break
					}
					sub = sub[:i]
				}
				return false
			}
			if searchVendor(ctxt.GOROOT, true) {
				goto Found
			}
			for _, root := range gopath {
				if searchVendor(root, false) {
					goto Found
				}
			}
		}

		// Determine directory from import path.
		if ctxt.GOROOT != "" {
			dir := ctxt.joinPath(ctxt.GOROOT, "src", path)
			isDir := ctxt.isDir(dir)
			binaryOnly = !isDir && mode&AllowBinary != 0 && pkga != "" && ctxt.isFile(ctxt.joinPath(ctxt.GOROOT, pkga))
			if isDir || binaryOnly {
				p.Dir = dir
				p.Goroot = true
				p.Root = ctxt.GOROOT
				goto Found
			}
			tried.goroot = dir
		}
		for _, root := range gopath {
			dir := ctxt.joinPath(root, "src", path)
			isDir := ctxt.isDir(dir)
			binaryOnly = !isDir && mode&AllowBinary != 0 && pkga != "" && ctxt.isFile(ctxt.joinPath(root, pkga))
			if isDir || binaryOnly {
				p.Dir = dir
				p.Root = root
				goto Found
			}
			tried.gopath = append(tried.gopath, dir)
		}

		// package was not found
		var paths []string
		format := "\t%s (vendor tree)"
		for _, dir := range tried.vendor {
			paths = append(paths, fmt.Sprintf(format, dir))
			format = "\t%s"
		}
		if tried.goroot != "" {
			paths = append(paths, fmt.Sprintf("\t%s (from $GOROOT)", tried.goroot))
		} else {
			paths = append(paths, "\t($GOROOT not set)")
		}
		format = "\t%s (from $GOPATH)"
		for _, dir := range tried.gopath {
			paths = append(paths, fmt.Sprintf(format, dir))
			format = "\t%s"
		}
		if len(tried.gopath) == 0 {
			paths = append(paths, "\t($GOPATH not set)")
		}
		return p, fmt.Errorf("cannot find package %q in any of:\n%s", path, strings.Join(paths, "\n"))
	}

Found:
	if p.Root != "" {
		p.SrcRoot = ctxt.joinPath(p.Root, "src")
		p.PkgRoot = ctxt.joinPath(p.Root, "pkg")
		p.BinDir = ctxt.joinPath(p.Root, "bin")
		if pkga != "" {
			p.PkgTargetRoot = ctxt.joinPath(p.Root, pkgtargetroot)
			p.PkgObj = ctxt.joinPath(p.Root, pkga)
		}
	}

	if mode&FindOnly != 0 {
		return p, pkgerr
	}
	if binaryOnly && (mode&AllowBinary) != 0 {
		return p, pkgerr
	}

	dirs, err := ctxt.readDir(p.Dir)
	if err != nil {
		return p, err
	}

	var badGoError error
	var Sfiles []string // files with ".S" (capital S)
	var firstFile, firstCommentFile string
	imported := make(map[string][]token.Position)
	testImported := make(map[string][]token.Position)
	xTestImported := make(map[string][]token.Position)
	allTags := make(map[string]bool)
	fset := token.NewFileSet()
	for _, d := range dirs {
		if d.IsDir() {
			continue
		}

		name := d.Name()
		ext := nameExt(name)

		badFile := func(err error) {
			if badGoError == nil {
				badGoError = err
			}
			// CEV
			// p.InvalidGoFiles = append(p.InvalidGoFiles, name)
		}

		match, data, filename, err := ctxt.matchFile(p.Dir, name, true, allTags)
		if err != nil {
			badFile(err)
			continue
		}
		if !match {
			if ext == ".go" {
				p.IgnoredGoFiles = append(p.IgnoredGoFiles, name)
			}
			continue
		}

		// Going to save the file.  For non-Go files, can stop here.
		switch ext {
		case ".c":
			p.CFiles = append(p.CFiles, name)
			continue
		case ".cc", ".cpp", ".cxx":
			p.CXXFiles = append(p.CXXFiles, name)
			continue
		case ".m":
			p.MFiles = append(p.MFiles, name)
			continue
		case ".h", ".hh", ".hpp", ".hxx":
			p.HFiles = append(p.HFiles, name)
			continue
		case ".s":
			p.SFiles = append(p.SFiles, name)
			continue
		case ".S":
			Sfiles = append(Sfiles, name)
			continue
		case ".swig":
			p.SwigFiles = append(p.SwigFiles, name)
			continue
		case ".swigcxx":
			p.SwigCXXFiles = append(p.SwigCXXFiles, name)
			continue
		case ".syso":
			// binary objects to add to package archive
			// Likely of the form foo_windows.syso, but
			// the name was vetted above with goodOSArchFile.
			p.SysoFiles = append(p.SysoFiles, name)
			continue
		}

		pf, err := parser.ParseFile(fset, filename, data, parser.ImportsOnly|parser.ParseComments)
		if err != nil {
			badFile(err)
			continue
		}

		pkg := pf.Name.Name
		if pkg == "documentation" {
			p.IgnoredGoFiles = append(p.IgnoredGoFiles, name)
			continue
		}

		isTest := strings.HasSuffix(name, "_test.go")
		isXTest := false
		if isTest && strings.HasSuffix(pkg, "_test") {
			isXTest = true
			pkg = pkg[:len(pkg)-len("_test")]
		}

		if p.Name == "" {
			p.Name = pkg
			firstFile = name
		} else if pkg != p.Name {
			badFile(&build.MultiplePackageError{
				Dir:      p.Dir,
				Packages: []string{p.Name, pkg},
				Files:    []string{firstFile, name},
			})
			// CEV
			// p.InvalidGoFiles = append(p.InvalidGoFiles, name)
		}
		if pf.Doc != nil && p.Doc == "" {
			p.Doc = doc.Synopsis(pf.Doc.Text())
		}

		if mode&ImportComment != 0 {
			qcom, line := findImportComment(data)
			if line != 0 {
				com, err := strconv.Unquote(qcom)
				if err != nil {
					badFile(fmt.Errorf("%s:%d: cannot parse import comment", filename, line))
				} else if p.ImportComment == "" {
					p.ImportComment = com
					firstCommentFile = name
				} else if p.ImportComment != com {
					badFile(fmt.Errorf("found import comments %q (%s) and %q (%s) in %s", p.ImportComment, firstCommentFile, com, name, p.Dir))
				}
			}
		}

		// Record imports and information about cgo.
		isCgo := false
		for _, decl := range pf.Decls {
			d, ok := decl.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, dspec := range d.Specs {
				spec, ok := dspec.(*ast.ImportSpec)
				if !ok {
					continue
				}
				quoted := spec.Path.Value
				path, err := strconv.Unquote(quoted)
				if err != nil {
					log.Panicf("%s: parser returned invalid quoted string: <%s>", filename, quoted)
				}
				if isXTest {
					xTestImported[path] = append(xTestImported[path], fset.Position(spec.Pos()))
				} else if isTest {
					testImported[path] = append(testImported[path], fset.Position(spec.Pos()))
				} else {
					imported[path] = append(imported[path], fset.Position(spec.Pos()))
				}
				if path == "C" {
					if isTest {
						badFile(fmt.Errorf("use of cgo in test %s not supported", filename))
					} else {
						cg := spec.Doc
						if cg == nil && len(d.Specs) == 1 {
							cg = d.Doc
						}
						if cg != nil {
							if err := ctxt.saveCgo(filename, p, cg); err != nil {
								badFile(err)
							}
						}
						isCgo = true
					}
				}
			}
		}
		if isCgo {
			allTags["cgo"] = true
			if ctxt.CgoEnabled {
				p.CgoFiles = append(p.CgoFiles, name)
			} else {
				p.IgnoredGoFiles = append(p.IgnoredGoFiles, name)
			}
		} else if isXTest {
			p.XTestGoFiles = append(p.XTestGoFiles, name)
		} else if isTest {
			p.TestGoFiles = append(p.TestGoFiles, name)
		} else {
			p.GoFiles = append(p.GoFiles, name)
		}
	}
	if badGoError != nil {
		return p, badGoError
	}
	if len(p.GoFiles)+len(p.CgoFiles)+len(p.TestGoFiles)+len(p.XTestGoFiles) == 0 {
		return p, &build.NoGoError{p.Dir}
	}

	for tag := range allTags {
		p.AllTags = append(p.AllTags, tag)
	}
	sort.Strings(p.AllTags)

	p.Imports, p.ImportPos = cleanImports(imported)
	p.TestImports, p.TestImportPos = cleanImports(testImported)
	p.XTestImports, p.XTestImportPos = cleanImports(xTestImported)

	// add the .S files only if we are using cgo
	// (which means gcc will compile them).
	// The standard assemblers expect .s files.
	if len(p.CgoFiles) > 0 {
		p.SFiles = append(p.SFiles, Sfiles...)
		sort.Strings(p.SFiles)
	}

	return p, pkgerr
}

// hasGoFiles reports whether dir contains any files with names ending in .go.
// For a vendor check we must exclude directories that contain no .go files.
// Otherwise it is not possible to vendor just a/b/c and still import the
// non-vendored a/b. See golang.org/issue/13832.
func hasGoFiles(ctxt *context, dir string) bool {
	ents, _ := ctxt.readDir(dir)
	for _, ent := range ents {
		if !ent.IsDir() && strings.HasSuffix(ent.Name(), ".go") {
			return true
		}
	}
	return false
}

func findImportComment(data []byte) (s string, line int) {
	// expect keyword package
	word, data := parseWord(data)
	if string(word) != "package" {
		return "", 0
	}

	// expect package name
	_, data = parseWord(data)

	// now ready for import comment, a // or /* */ comment
	// beginning and ending on the current line.
	for len(data) > 0 && (data[0] == ' ' || data[0] == '\t' || data[0] == '\r') {
		data = data[1:]
	}

	var comment []byte
	switch {
	case bytes.HasPrefix(data, slashSlash):
		i := bytes.Index(data, newline)
		if i < 0 {
			i = len(data)
		}
		comment = data[2:i]
	case bytes.HasPrefix(data, slashStar):
		data = data[2:]
		i := bytes.Index(data, starSlash)
		if i < 0 {
			// malformed comment
			return "", 0
		}
		comment = data[:i]
		if bytes.Contains(comment, newline) {
			return "", 0
		}
	}
	comment = bytes.TrimSpace(comment)

	// split comment into `import`, `"pkg"`
	word, arg := parseWord(comment)
	if string(word) != "import" {
		return "", 0
	}

	line = 1 + bytes.Count(data[:cap(data)-cap(arg)], newline)
	return strings.TrimSpace(string(arg)), line
}

var (
	slashSlash = []byte("//")
	slashStar  = []byte("/*")
	starSlash  = []byte("*/")
	newline    = []byte("\n")
)

// skipSpaceOrComment returns data with any leading spaces or comments removed.
func skipSpaceOrComment(data []byte) []byte {
	for len(data) > 0 {
		switch data[0] {
		case ' ', '\t', '\r', '\n':
			data = data[1:]
			continue
		case '/':
			if bytes.HasPrefix(data, slashSlash) {
				i := bytes.Index(data, newline)
				if i < 0 {
					return nil
				}
				data = data[i+1:]
				continue
			}
			if bytes.HasPrefix(data, slashStar) {
				data = data[2:]
				i := bytes.Index(data, starSlash)
				if i < 0 {
					return nil
				}
				data = data[i+2:]
				continue
			}
		}
		break
	}
	return data
}

// parseWord skips any leading spaces or comments in data
// and then parses the beginning of data as an identifier or keyword,
// returning that word and what remains after the word.
func parseWord(data []byte) (word, rest []byte) {
	data = skipSpaceOrComment(data)

	// Parse past leading word characters.
	rest = data
	for {
		r, size := utf8.DecodeRune(rest)
		if unicode.IsLetter(r) || '0' <= r && r <= '9' || r == '_' {
			rest = rest[size:]
			continue
		}
		break
	}

	word = data[:len(data)-len(rest)]
	if len(word) == 0 {
		return nil, nil
	}

	return word, rest
}

// MatchFile reports whether the file with the given name in the given directory
// matches the context and would be included in a Package created by ImportDir
// of that directory.
//
// MatchFile considers the name of the file and may use ctxt.OpenFile to
// read some or all of the file's content.
func MatchFile(ctxt *build.Context, dir, name string) (match bool, err error) {
	match, _, _, err = newContext(ctxt).matchFile(dir, name, false, nil)
	return
}

// matchFile determines whether the file with the given name in the given directory
// should be included in the package being constructed.
// It returns the data read from the file.
// If returnImports is true and name denotes a Go program, matchFile reads
// until the end of the imports (and returns that data) even though it only
// considers text until the first non-comment.
// If allTags is non-nil, matchFile records any encountered build tag
// by setting allTags[tag] = true.
func (ctxt *context) matchFile(dir, name string, returnImports bool, allTags map[string]bool) (match bool, data []byte, filename string, err error) {
	if strings.HasPrefix(name, "_") ||
		strings.HasPrefix(name, ".") {
		return
	}

	i := strings.LastIndex(name, ".")
	if i < 0 {
		i = len(name)
	}
	ext := name[i:]

	if !ctxt.goodOSArchFile(name, allTags) && !ctxt.UseAllFiles {
		return
	}

	switch ext {
	case ".go", ".c", ".cc", ".cxx", ".cpp", ".m", ".s", ".h", ".hh", ".hpp", ".hxx", ".S", ".swig", ".swigcxx":
		// tentatively okay - read to make sure
	case ".syso":
		// binary, no reading
		match = true
		return
	default:
		// skip
		return
	}

	filename = ctxt.joinPath(dir, name)
	f, err := ctxt.openFile(filename)
	if err != nil {
		return
	}

	if strings.HasSuffix(filename, ".go") {
		data, err = readImports(f, false, nil)
	} else {
		data, err = readComments(f)
	}
	f.Close()
	if err != nil {
		err = fmt.Errorf("read %s: %v", filename, err)
		return
	}

	// Look for +build comments to accept or reject the file.
	if !ctxt.shouldBuild(data, allTags) && !ctxt.UseAllFiles {
		return
	}

	match = true
	return
}

func cleanImports(m map[string][]token.Position) ([]string, map[string][]token.Position) {
	all := make([]string, 0, len(m))
	for path := range m {
		all = append(all, path)
	}
	sort.Strings(all)
	return all, m
}

func (ctxt *context) shouldBuild(content []byte, allTags map[string]bool) bool {
	return shouldBuild(&ctxt.Context, content, allTags)
}

// saveCgo saves the information from the #cgo lines in the import "C" comment.
// These lines set CFLAGS, CPPFLAGS, CXXFLAGS and LDFLAGS and pkg-config directives
// that affect the way cgo's C code is built.
func (ctxt *context) saveCgo(filename string, di *build.Package, cg *ast.CommentGroup) error {
	text := cg.Text()
	for _, line := range strings.Split(text, "\n") {
		orig := line

		// Line is
		//	#cgo [GOOS/GOARCH...] LDFLAGS: stuff
		//
		line = strings.TrimSpace(line)
		if len(line) < 5 || line[:4] != "#cgo" || (line[4] != ' ' && line[4] != '\t') {
			continue
		}

		// Split at colon.
		line = strings.TrimSpace(line[4:])
		i := strings.Index(line, ":")
		if i < 0 {
			return fmt.Errorf("%s: invalid #cgo line: %s", filename, orig)
		}
		line, argstr := line[:i], line[i+1:]

		// Parse GOOS/GOARCH stuff.
		f := strings.Fields(line)
		if len(f) < 1 {
			return fmt.Errorf("%s: invalid #cgo line: %s", filename, orig)
		}

		cond, verb := f[:len(f)-1], f[len(f)-1]
		if len(cond) > 0 {
			ok := false
			for _, c := range cond {
				if ctxt.match(c, nil) {
					ok = true
					break
				}
			}
			if !ok {
				continue
			}
		}

		args, err := splitQuoted(argstr)
		if err != nil {
			return fmt.Errorf("%s: invalid #cgo line: %s", filename, orig)
		}
		var ok bool
		for i, arg := range args {
			if arg, ok = expandSrcDir(arg, di.Dir); !ok {
				return fmt.Errorf("%s: malformed #cgo argument: %s", filename, arg)
			}
			args[i] = arg
		}

		switch verb {
		case "CFLAGS":
			di.CgoCFLAGS = append(di.CgoCFLAGS, args...)
		case "CPPFLAGS":
			di.CgoCPPFLAGS = append(di.CgoCPPFLAGS, args...)
		case "CXXFLAGS":
			di.CgoCXXFLAGS = append(di.CgoCXXFLAGS, args...)
		case "LDFLAGS":
			di.CgoLDFLAGS = append(di.CgoLDFLAGS, args...)
		case "pkg-config":
			di.CgoPkgConfig = append(di.CgoPkgConfig, args...)
		default:
			return fmt.Errorf("%s: invalid #cgo verb: %s", filename, orig)
		}
	}
	return nil
}

// expandSrcDir expands any occurrence of ${SRCDIR}, making sure
// the result is safe for the shell.
func expandSrcDir(str string, srcdir string) (string, bool) {
	// "\" delimited paths cause safeCgoName to fail
	// so convert native paths with a different delimeter
	// to "/" before starting (eg: on windows).
	srcdir = filepath.ToSlash(srcdir)

	// Spaces are tolerated in ${SRCDIR}, but not anywhere else.
	chunks := strings.Split(str, "${SRCDIR}")
	if len(chunks) < 2 {
		return str, safeCgoName(str, false)
	}
	ok := true
	for _, chunk := range chunks {
		ok = ok && (chunk == "" || safeCgoName(chunk, false))
	}
	ok = ok && (srcdir == "" || safeCgoName(srcdir, true))
	res := strings.Join(chunks, srcdir)
	return res, ok && res != ""
}

// NOTE: $ is not safe for the shell, but it is allowed here because of linker options like -Wl,$ORIGIN.
// We never pass these arguments to a shell (just to programs we construct argv for), so this should be okay.
// See golang.org/issue/6038.
// The @ is for OS X. See golang.org/issue/13720.
const safeString = "+-.,/0123456789=ABCDEFGHIJKLMNOPQRSTUVWXYZ_abcdefghijklmnopqrstuvwxyz:$@"
const safeSpaces = " "

var safeBytes = []byte(safeSpaces + safeString)

func safeCgoName(s string, spaces bool) bool {
	if s == "" {
		return false
	}
	safe := safeBytes
	if !spaces {
		safe = safe[len(safeSpaces):]
	}
	for i := 0; i < len(s); i++ {
		if c := s[i]; c < 0x80 && bytes.IndexByte(safe, c) < 0 {
			return false
		}
	}
	return true
}

// splitQuoted splits the string s around each instance of one or more consecutive
// white space characters while taking into account quotes and escaping, and
// returns an array of substrings of s or an empty list if s contains only white space.
// Single quotes and double quotes are recognized to prevent splitting within the
// quoted region, and are removed from the resulting substrings. If a quote in s
// isn't closed err will be set and r will have the unclosed argument as the
// last element.  The backslash is used for escaping.
//
// For example, the following string:
//
//     a b:"c d" 'e''f'  "g\""
//
// Would be parsed as:
//
//     []string{"a", "b:c d", "ef", `g"`}
//
func splitQuoted(s string) (r []string, err error) {
	var args []string
	arg := make([]rune, len(s))
	escaped := false
	quoted := false
	quote := '\x00'
	i := 0
	for _, rune := range s {
		switch {
		case escaped:
			escaped = false
		case rune == '\\':
			escaped = true
			continue
		case quote != '\x00':
			if rune == quote {
				quote = '\x00'
				continue
			}
		case rune == '"' || rune == '\'':
			quoted = true
			quote = rune
			continue
		case unicode.IsSpace(rune):
			if quoted || i > 0 {
				quoted = false
				args = append(args, string(arg[:i]))
				i = 0
			}
			continue
		}
		arg[i] = rune
		i++
	}
	if quoted || i > 0 {
		args = append(args, string(arg[:i]))
	}
	if quote != 0 {
		err = errors.New("unclosed quote")
	} else if escaped {
		err = errors.New("unfinished escaping")
	}
	return args, err
}

func (ctxt *context) match(name string, allTags map[string]bool) bool {
	return match(&ctxt.Context, name, allTags)
}

func (ctxt *context) goodOSArchFile(name string, allTags map[string]bool) bool {
	return goodOSArchFile(&ctxt.Context, name, allTags)
}
