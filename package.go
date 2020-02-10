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
	"github.com/ghetzel/go-stockutil/mathutil"
	"github.com/ghetzel/go-stockutil/rxutil"
	"github.com/ghetzel/go-stockutil/sliceutil"
	"github.com/ghetzel/go-stockutil/stringutil"
	"github.com/montanaflynn/stats"
	"golang.org/x/tools/go/vcs"
)

// These numbers represent a hypothetical "ideal" word count for various kinds
// of Golang statements.  It was arrived at via entirely subjective means (These
// numbers "felt right" to me at the time of writing [2020-02-10].)

// Ideal number of words per struct declaration.
var IdealWordCountStruct float64 = 15

// Ideal number of words used to describe struct fields.
var IdealWordCountStructField float64 = 10

// Ideal number of words used to describe package variables.
var IdealWordCountVar float64 = 8

// Ideal number of words used to describe function and struct method definitions.
var IdealWordCountFunc float64 = 10

var MaxExpressionSnippetLength = 64

// Represents statistical rollups of sets of numbers.
type Rollup struct {
	Mean          float64
	StdDev        float64
	GeometricMean float64
	HarmonicMean  float64
	Median        float64
	Minimum       float64
	Maximum       float64
}

type Package struct {
	Name                string
	CanonicalImportPath string
	ImportPath          string
	ParentPackage       string
	URL                 string
	Synopsis            string
	Files               []*File
	Constants           []Value          `json:",omitempty"`
	Variables           []Value          `json:",omitempty"`
	Functions           []*Method        `json:",omitempty"`
	Examples            []*Method        `json:",omitempty"`
	Tests               []*Method        `json:",omitempty"`
	Types               map[string]*Type `json:",omitempty"`
	Packages            []*Package       `json:",omitempty"`
	CommentWordCount    int
	LineCount           int
	SourceLineCount     int
	FunctionCount       int
	TypeCount           int
	ConstantCount       int
	VariableCount       int
	Statistics          Rollup
	ast                 *ast.Package
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

	var structTypeCoverage []float64
	var structFieldCoverage []float64
	var funcCoverage []float64
	var varCoverage []float64

	for _, file := range self.Files {
		self.LineCount += file.LineCount
		self.SourceLineCount += file.SourceLineCount
		self.FunctionCount += file.FunctionCount
		self.TypeCount += file.TypeCount
		self.ConstantCount += file.ConstantCount
		self.VariableCount += file.VariableCount
	}

	for _, typ := range self.Types {
		wc := wordcount(typ.Comment)
		self.CommentWordCount += wc

		if typ.MetaType == `struct` {
			structTypeCoverage = append(structTypeCoverage, mathutil.ClampUpper(1.0, (float64(wc)/IdealWordCountStruct)))
		}

		for _, m := range typ.Methods {
			wc := wordcount(m.Comment)
			self.CommentWordCount += wc
			funcCoverage = append(funcCoverage, mathutil.ClampUpper(1.0, (float64(wc)/IdealWordCountFunc)))
		}

		for _, f := range typ.Fields {
			wc := wordcount(f.Comment)
			self.CommentWordCount += wc
			structFieldCoverage = append(structFieldCoverage, mathutil.ClampUpper(1.0, (float64(wc)/IdealWordCountStructField)))
		}
	}

	for _, f := range self.Functions {
		wc := wordcount(f.Comment)
		self.CommentWordCount += wc
		funcCoverage = append(funcCoverage, mathutil.ClampUpper(1.0, (float64(wc)/IdealWordCountFunc)))
	}

	for _, v := range self.Variables {
		wc := wordcount(v.Comment)
		self.CommentWordCount += wc
		varCoverage = append(varCoverage, mathutil.ClampUpper(1.0, (float64(wc)/IdealWordCountVar)))
	}

	var agg stats.Float64Data

	agg = append(agg, structTypeCoverage...)
	agg = append(agg, structFieldCoverage...)
	agg = append(agg, funcCoverage...)
	agg = append(agg, varCoverage...)

	if v, err := agg.Mean(); err == nil {
		self.Statistics.Mean = v
	}

	if v, err := agg.StandardDeviation(); err == nil {
		self.Statistics.StdDev = v
	}

	if v, err := agg.GeometricMean(); err == nil {
		self.Statistics.GeometricMean = v
	}

	if v, err := agg.HarmonicMean(); err == nil {
		self.Statistics.HarmonicMean = v
	}

	if v, err := agg.Median(); err == nil {
		self.Statistics.Median = v
	}

	if v, err := agg.Min(); err == nil {
		self.Statistics.Minimum = v
	}

	if v, err := agg.Max(); err == nil {
		self.Statistics.Maximum = v
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
	return loadPackage(parentDir, ``)
}

func loadPackage(pkgdir string, parentName string) (*Package, error) {
	log.Infof("load package from: %s", pkgdir)
	fset := token.NewFileSet()

	if pkgs, err := parser.ParseDir(
		fset,
		pkgdir,
		nil,
		(parser.ParseComments | parser.DeclarationErrors | parser.AllErrors),
	); err == nil {
		for _, pkg := range pkgs {
			pkgDoc := doc.New(pkg, pkgdir, doc.PreserveAST)

			p := new(Package)
			p.ast = pkg
			p.Name = pkgDoc.Name
			p.Synopsis = pkgDoc.Doc
			p.ImportPath = pkgDoc.ImportPath
			p.CanonicalImportPath = p.ImportPath
			p.ParentPackage = parentName

			if abs, _ := filepath.Abs(pkgdir); err == nil {
				if deducedImportBase, err := GetImportPathFromDir(abs, locateSourceRoot(abs)); err == nil {
					if p.ImportPath == `.` {
						p.ImportPath = p.Name
						p.CanonicalImportPath = deducedImportBase
					} else {
						p.CanonicalImportPath = filepath.Join(deducedImportBase, p.ImportPath)
					}

					if rroot, err := vcs.RepoRootForImportPath(p.CanonicalImportPath, false); err == nil {
						p.URL = rroot.Repo
					} else {
						return nil, fmt.Errorf("bad url: %v", err)
					}
				} else {
					return nil, fmt.Errorf("bad import path: %v", err)
				}
			} else {
				return nil, fmt.Errorf("bad path: %v", err)
			}

			p.Functions = make([]*Method, 0)
			p.Types = make(map[string]*Type)
			p.Files = make([]*File, 0)

			for fname, f := range pkg.Files {
				if err := p.addFile(fname, f); err != nil {
					return nil, err
				}
			}

			if entries, err := ioutil.ReadDir(pkgdir); err == nil {
				for _, entry := range entries {
					if entry.IsDir() {
						path := filepath.Join(pkgdir, entry.Name())

						if subpkg, err := loadPackage(path, p.ImportPath); err == nil {
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

func GetImportPathFromDir(dir string, srcRoot string) (string, error) {
	if _, root, err := vcs.FromDir(dir, srcRoot); err == nil {
		return root, nil
	} else {
		return ``, err
	}
}

func locateSourceRoot(dir string) string {
	switch filepath.Base(dir) {
	case `.`, ``:
		return ``
	case `src`:
		return dir
	default:
		return locateSourceRoot(filepath.Dir(dir))
	}
}
