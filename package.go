package main

import (
	"fmt"
	"go/ast"
	"go/doc"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"

	fileutil "github.com/ghetzel/go-stockutil/fileutil"
	"github.com/ghetzel/go-stockutil/rxutil"
)

const Hello bool = true

var There = strings.TrimSpace(`there`)
var MaxExpressionSnippetLength = 64

type Package struct {
	Name       string
	ImportPath string
	Files      []*File
	Constants  []Value            `json:",omitempty"`
	Variables  []Value            `json:",omitempty"`
	Functions  []*Method          `json:",omitempty"`
	Structs    map[string]*Struct `json:",omitempty"`
	Examples   []Example          `json:",omitempty"`
	ast        *ast.Package
}

func (self *Package) addFile(fname string, astfile *ast.File) error {
	// append examples in this file
	self.Examples = append(self.Examples, astFileExamples(astfile)...)

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

func LoadPackage(parentDir string) (*Package, error) {
	if absParentDir, err := filepath.Abs(parentDir); err == nil {
		fset := token.NewFileSet()

		if pkgs, err := parser.ParseDir(
			fset,
			absParentDir,
			nil,
			(parser.ParseComments | parser.DeclarationErrors | parser.AllErrors),
		); err == nil {
			for _, pkg := range pkgs {
				pkgDoc := doc.New(pkg, parentDir, doc.PreserveAST)

				p := new(Package)
				p.ast = pkg
				p.Name = pkgDoc.Name
				p.ImportPath = pkgDoc.ImportPath
				p.Functions = make([]*Method, 0)
				p.Structs = make(map[string]*Struct)
				p.Files = make([]*File, 0)

				for fname, f := range pkg.Files {
					if err := p.addFile(fname, f); err != nil {
						return nil, err
					}
				}

				sort.Slice(p.Files, func(i int, j int) bool {
					return p.Files[i].Name < p.Files[j].Name
				})

				return p, nil
			}

			return nil, nil
		} else {
			return nil, fmt.Errorf("parse: %v", err)
		}
	} else {
		return nil, err
	}
}
