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
	"github.com/ghetzel/go-stockutil/sliceutil"
	"github.com/ghetzel/go-stockutil/stringutil"
)

const Hello bool = true

var There = strings.TrimSpace(`there`)
var MaxExpressionSnippetLength = 64

type Package struct {
	Name             string
	ImportPath       string
	Synopsis         string
	Files            []*File
	Constants        []Value          `json:",omitempty"`
	Variables        []Value          `json:",omitempty"`
	Functions        []*Method        `json:",omitempty"`
	Examples         []*Method        `json:",omitempty"`
	Tests            []*Method        `json:",omitempty"`
	Types            map[string]*Type `json:",omitempty"`
	Packages         []*Package       `json:",omitempty"`
	CommentWordCount int
	LineCount        int
	SourceLineCount  int
	FunctionCount    int
	TypeCount        int
	ConstantCount    int
	VariableCount    int
	ast              *ast.Package
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
			self.recalcTotals()
			return nil
		} else {
			return fmt.Errorf("%s: %v", file.Name, err)
		}
	} else {
		return fmt.Errorf("unreadable source %q: %v", fname, err)
	}
}

func (self *Package) recalcTotals() {
	self.CommentWordCount = 0
	self.LineCount = 0
	self.SourceLineCount = 0
	self.FunctionCount = 0
	self.TypeCount = 0
	self.ConstantCount = 0
	self.VariableCount = 0

	for _, file := range self.Files {
		self.LineCount += file.LineCount
		self.SourceLineCount += file.SourceLineCount
		self.FunctionCount += file.FunctionCount
		self.TypeCount += file.TypeCount
		self.ConstantCount += file.ConstantCount
		self.VariableCount += file.VariableCount
	}

	for _, typ := range self.Types {
		self.CommentWordCount += wordcount(typ.Comment)

		for _, m := range typ.Methods {
			self.CommentWordCount += wordcount(m.Comment)
		}

		for _, f := range typ.Fields {
			self.CommentWordCount += wordcount(f.Comment)
		}
	}

	for _, f := range self.Functions {
		self.CommentWordCount += wordcount(f.Comment)
	}

	for _, v := range self.Variables {
		self.CommentWordCount += wordcount(v.Comment)
	}

	self.CommentWordCount += wordcount(self.Synopsis)
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
	log.Infof("load package from: %s", parentDir)
	fset := token.NewFileSet()

	if pkgs, err := parser.ParseDir(
		fset,
		parentDir,
		nil,
		(parser.ParseComments | parser.DeclarationErrors | parser.AllErrors),
	); err == nil {
		for _, pkg := range pkgs {
			pkgDoc := doc.New(pkg, parentDir, doc.PreserveAST)
			// log.Dump(pkgDoc)

			p := new(Package)
			p.ast = pkg
			p.Name = pkgDoc.Name
			p.Synopsis = pkgDoc.Doc
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
			p.recalcTotals()

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

func wordcount(s string) int {
	words := stringutil.SplitWords(s)
	words = sliceutil.CompactString(words)
	return len(words)
}
