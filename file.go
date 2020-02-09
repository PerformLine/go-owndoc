package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/doc"
	"go/format"
	"go/token"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ghetzel/go-stockutil/log"
	"github.com/ghetzel/go-stockutil/stringutil"
)

type Example struct {
	Name    string
	Comment string
	Output  string
}

// Represents a constant or variable declaration.
type Value struct {
	Name       string
	Type       string `json:",omitempty"`
	Immutable  bool   `json:",omitempty"`
	Expression string `json:",omitempty"`
}

// Represents a function argument or return value.
type Arg struct {
	Name string `json:",omitempty"`
	Type string
}

// Represents a function declaration, both package-level and struct methods.
type Method struct {
	// The parent struct this method is attached to.
	Struct *Struct `json:"-"`

	// The File this declaration appears in.
	File *File `json:"-"`

	// The name of the method.
	Name string

	// The comment text describing the function.
	Comment string `json:",omitempty"`

	// Whether this method is attached to a Struct instance or Struct pointer.
	PointerReceiver bool `json:",omitempty"`

	// The variable used to access the receiver instance.
	ReceiverName string `json:",omitempty"`

	// List of arguments this method accepts.
	Arguments []Arg `json:",omitempty"`

	// List of outputs this method returns.
	Returns []Arg `json:",omitempty"`

	// Return a source representation of the function signature
	Signature string `json:",omitempty"`
}

// Represents a single field in a struct declaration.
type Field struct {
	Name    string
	Type    string
	Struct  *Struct `json:"-"`
	Comment string  `json:",omitempty"`
}

// Represents a struct, including all of its constituent fields and methods.
type Struct struct {
	File    *File `json:"-"`
	Name    string
	Fields  []*Field
	Methods []*Method `json:",omitempty"`
	Comment string    `json:",omitempty"`
}

// Represents an import declaration for a dependent package.
type Import struct {
	PackageName string
	Alias       string `json:",omitempty"`
}

// Represents a Golang source file.
type File struct {
	Name            string
	Package         *Package `json:"-"`
	Imports         []Import
	MainFunction    bool `json:",omitempty"`
	Size            int64
	LineCount       int
	SourceLineCount int
	ast             *ast.File
}

func (self *File) parse() error {
	if self.ast == nil {
		return fmt.Errorf("cannot parse nil AST")
	}

	for _, importSpec := range self.ast.Imports {
		var imp Import

		imp.PackageName = importSpec.Path.Value
		imp.PackageName = stringutil.Unwrap(imp.PackageName, `"`, `"`)
		imp.PackageName = strings.TrimSpace(imp.PackageName)

		if importSpec.Name == nil {
			imp.Alias = filepath.Base(imp.PackageName)
		} else {
			imp.Alias = importSpec.Name.String()
		}

		self.Imports = append(self.Imports, imp)
	}

	for _, decl := range self.ast.Decls {
		switch decl.(type) {
		case *ast.FuncDecl:
			self.appendFuncDecl(decl.(*ast.FuncDecl))
		case *ast.GenDecl:
			gen := decl.(*ast.GenDecl)

			for _, spec := range gen.Specs {
				switch spec.(type) {
				case *ast.ValueSpec: // consts and vars
					vspec := spec.(*ast.ValueSpec)
					value := Value{
						Name: vspec.Names[0].String(),
						Type: astTypeToString(vspec.Type),
					}

					for _, val := range vspec.Values {
						if expr := strings.TrimSpace(mustAstNodeToString(val)); len(expr) <= MaxExpressionSnippetLength {
							value.Expression = expr
						}

						break
					}

					switch gen.Tok {
					case token.CONST:
						value.Immutable = true
						self.Package.Constants = append(self.Package.Constants, value)
					case token.VAR:
						self.Package.Variables = append(self.Package.Variables, value)
					}

				case *ast.TypeSpec: // type declarations
					tspec := spec.(*ast.TypeSpec)

					switch tspec.Type.(type) {
					case *ast.StructType: // structs
						self.appendStruct(
							gen,
							tspec.Name.Name,
							tspec.Type.(*ast.StructType),
						)
					}
				}
			}
		default:
			log.Debugf("%s: unhandled: %T", self.Name, decl)
		}
	}

	sort.Slice(self.Package.Functions, func(i int, j int) bool {
		return self.Package.Functions[i].Name < self.Package.Functions[j].Name
	})

	return nil
}

func (self *File) appendFuncDecl(fn *ast.FuncDecl) {
	method := new(Method)
	method.File = self
	method.Name = fn.Name.Name
	method.Comment = formatAstComment(fn.Doc)

	if ast.IsExported(method.Name) {
		defer func(m *Method) {
			var argset []string
			var retset []string

			for _, arg := range m.Arguments {
				argset = append(argset, fmt.Sprintf("%v %v", arg.Name, arg.Type))
			}

			for _, ret := range m.Returns {
				if ret.Name != `` {
					retset = append(retset, fmt.Sprintf("%v %v", ret.Name, ret.Type))
				} else {
					retset = append(retset, ret.Type)
				}
			}

			m.Signature = m.Name + `(` + strings.Join(argset, `, `) + `)`

			switch len(retset) {
			case 0:
				return
			case 1:
				m.Signature += ` ` + retset[0]
			default:
				m.Signature += ` (` + strings.Join(retset, `, `) + `)`
			}
		}(method)

		for _, param := range fn.Type.Params.List {
			var name string

			if len(param.Names) > 0 {
				name = param.Names[0].String()
			}

			method.Arguments = append(method.Arguments, Arg{
				Name: name,
				Type: astTypeToString(param.Type),
			})
		}

		if fn.Type.Results != nil {
			for _, res := range fn.Type.Results.List {
				var name string

				if len(res.Names) > 0 {
					name = res.Names[0].String()
				}

				method.Returns = append(method.Returns, Arg{
					Name: name,
					Type: astTypeToString(res.Type),
				})
			}
		}

		// no receiver == package-level function
		if fn.Recv == nil {
			self.Package.Functions = append(self.Package.Functions, method)
			return
		} else if len(fn.Recv.List) > 0 {
			var listField = fn.Recv.List[len(fn.Recv.List)-1]
			var listFieldType = listField.Type
			var ident *ast.Ident

			if len(listField.Names) > 0 {
				method.ReceiverName = listField.Names[0].Name
			}

			switch listFieldType.(type) {
			case *ast.StarExpr: // e.g.: func (doc *Whatever) Hello()
				method.PointerReceiver = true
				ident = listFieldType.(*ast.StarExpr).X.(*ast.Ident)
			case *ast.Ident: // e.g.: func (doc Whatever) Hello()
				ident = listFieldType.(*ast.Ident)
			}

			if ident != nil {
				if recvName := ident.Name; ast.IsExported(recvName) {

					var parent *Struct = self.Package.Structs[recvName]

					if parent == nil {
						parent = new(Struct)
						parent.Name = recvName
					}

					parent.Methods = append(parent.Methods, method)
					self.Package.Structs[recvName] = parent
				} else {
					return
				}
			}
		}
	} else if method.Name == `main` {
		self.MainFunction = true
	}
}

func (self *File) appendStruct(meta *ast.GenDecl, name string, typ *ast.StructType) {
	if ast.IsExported(name) {
		var strct *Struct

		if s, ok := self.Package.Structs[name]; ok {
			strct = s
		} else {
			strct = new(Struct)
		}

		strct.Name = name
		strct.Fields = make([]*Field, 0)
		strct.Comment = formatAstComment(meta.Doc)

		for _, field := range typ.Fields.List {
			if len(field.Names) > 0 {
				if fieldName := field.Names[0].String(); ast.IsExported(fieldName) {
					strct.Fields = append(strct.Fields, &Field{
						Name:    fieldName,
						Type:    astTypeToString(field.Type),
						Comment: formatAstComment(field.Doc),
					})
				}
			}
		}

		self.Package.Structs[name] = strct
	}
}

func astTypeToString(typ ast.Expr) string {
	switch typ.(type) {
	case *ast.Ident:
		return typ.(*ast.Ident).String()
	case *ast.ArrayType:
		return `[]` + astTypeToString(typ.(*ast.ArrayType).Elt)
	case *ast.StarExpr:
		return `*` + astTypeToString(typ.(*ast.StarExpr).X)
	case *ast.MapType:
		mt := typ.(*ast.MapType)
		return `map[` + astTypeToString(mt.Key) + `]` + astTypeToString(mt.Value)
	default:
		return ``
	}
}

func formatAstComment(doc *ast.CommentGroup) string {
	if doc != nil {
		return strings.TrimSpace(doc.Text())
	}

	return ``
}

func astFileExamples(file *ast.File) (examples []Example) {
	for _, ex := range doc.Examples(file) {
		examples = append(examples, Example{
			Name:    ex.Name,
			Comment: ex.Doc,
			Output:  ex.Output,
		})
	}

	return
}

func mustAstNodeToString(node ast.Node) string {
	var buf bytes.Buffer
	var fset = token.NewFileSet()

	if err := format.Node(&buf, fset, node); err == nil {
		return buf.String()
	} else {
		panic(err.Error())
	}
}
