package main

import (
	"fmt"
	"go/ast"
	"go/doc"
	"go/parser"
	"go/token"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"

	fileutil "github.com/ghetzel/go-stockutil/fileutil"
	"github.com/ghetzel/go-stockutil/log"
	"github.com/ghetzel/go-stockutil/rxutil"
)

const Hello bool = true

var There = strings.TrimSpace(`there`)
var MaxExpressionSnippetLength = 64

type Package struct {
	Name       string
	ImportPath string
	Files      []*File
	Constants  []Value          `json:",omitempty"`
	Variables  []Value          `json:",omitempty"`
	Functions  []*Method        `json:",omitempty"`
	Examples   []*Method        `json:",omitempty"`
	Tests      []*Method        `json:",omitempty"`
	Types      map[string]*Type `json:",omitempty"`
	Packages   []*Package       `json:",omitempty"`
	ast        *ast.Package
}

func (self *Package) addFile(fname string, astfile *ast.File) error {
	// // append examples in this file
	// self.Examples = append(self.Examples, astFileExamples(astfile)...)

	if stat, err := os.Stat(fname); err == nil {
		file := &File{
			Name:    filepath.Base(fname),
			Package: self,
			Size:    stat.Size(),
			ast:     astfile,
		}

		for _, line := range fileutil.MustReadAllLines(fname) {
			file.LineCount += 1

			if rxutil.IsMatchString(`^\s*(?:\/\/.*)?$`, line) {
				continue
			}

			file.SourceLineCount += 1
		}

		if err := file.parse(); err == nil {
			self.Files = append(self.Files, file)
			return nil
		} else {
			return fmt.Errorf("%s: %v", file.Name, err)
		}
	} else {
		return fmt.Errorf("unreadable source %q: %v", fname, err)
	}
}

func (self *Package) sortObjects() {
	sort.Slice(self.Files, func(i int, j int) bool {
		return self.Files[i].Name < self.Files[j].Name
	})

	sort.Slice(self.Constants, func(i int, j int) bool {
		return self.Constants[i].Name < self.Constants[j].Name
	})

	sort.Slice(self.Variables, func(i int, j int) bool {
		return self.Variables[i].Name < self.Variables[j].Name
	})

	sort.Slice(self.Functions, func(i int, j int) bool {
		return self.Functions[i].Name < self.Functions[j].Name
	})

	sort.Slice(self.Examples, func(i int, j int) bool {
		return self.Examples[i].Name < self.Examples[j].Name
	})

	sort.Slice(self.Tests, func(i int, j int) bool {
		return self.Tests[i].Name < self.Tests[j].Name
	})
}

func LoadPackage(parentDir string) (*Package, error) {
	fset := token.NewFileSet()

	if pkgs, err := parser.ParseDir(
		fset,
		parentDir,
		nil,
		(parser.ParseComments | parser.DeclarationErrors | parser.AllErrors),
	); err == nil {
		for _, pkg := range pkgs {
			pkgDoc := doc.New(pkg, parentDir, doc.PreserveAST)
			log.Dump(pkgDoc)

			p := new(Package)
			p.ast = pkg
			p.Name = pkgDoc.Name
			p.ImportPath = pkgDoc.ImportPath
			p.Functions = make([]*Method, 0)
			p.Types = make(map[string]*Type)
			p.Files = make([]*File, 0)

			for fname, f := range pkg.Files {
				if err := p.addFile(fname, f); err != nil {
					return nil, err
				}
			}

			if entries, err := ioutil.ReadDir(parentDir); err == nil {
				for _, entry := range entries {
					if entry.IsDir() {
						path := filepath.Join(parentDir, entry.Name())

						if subpkg, err := LoadPackage(path); err == nil {
							if subpkg != nil {
								p.Packages = append(p.Packages, subpkg)
							}
						} else {
							return nil, fmt.Errorf("dir %s: %v", path, err)
						}
					}
				}
			} else {
				return nil, err
			}

			p.sortObjects()

			return p, nil
		}

		return nil, nil
	} else {
		return nil, fmt.Errorf("parse: %v", err)
	}
}

func parseFilterGoNoTests(stat os.FileInfo) bool {
	filename := stat.Name()
	filename = strings.ToLower(filename)

	if strings.HasSuffix(filename, `.go`) {
		if strings.HasSuffix(filename, `_test.go`) {
			return false
		} else {
			return true
		}
	}

	return false
}
