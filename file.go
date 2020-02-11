package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ghetzel/go-stockutil/log"
	"github.com/ghetzel/go-stockutil/stringutil"
	"github.com/ghetzel/go-stockutil/typeutil"
)

const CommentExportedFields = `// contains filtered or unexported fields`

// Represents a constant or variable declaration.
type Value struct {
	Name       string
	Type       string `json:",omitempty"`
	Immutable  bool   `json:",omitempty"`
	Expression string `json:",omitempty"`
	Value      string `json:",omitempty"`
	Comment    string `json:",omitempty"`
}

// Represents a function argument or return value.
type Arg struct {
	Name string `json:",omitempty"`
	Type string
}

// Represents a function declaration, both package-level and struct methods.
type Method struct {
	// The parent struct this method is attached to.
	Parent *Type `json:"-"`

	// The File this declaration appears in.
	File *File `json:"-"`

	// The name of the method.
	Name string

	// The name of an example method, as extracted from the method name.
	Label string `json:",omitempty"`

	// The name of the function the example is for.
	For string `json:",omitempty"`

	// The expected output of the method (for examples)
	ExpectedOutput string `json:",omitempty"`

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

	// Optional full text of the function's source.
	Source string `json:",omitempty"`

	// Return whether this is a package-level function or struct method.
	IsPackageLevel bool
}

// Represents a single field in a struct declaration.
type Field struct {
	Name    string
	Type    string
	Parent  *Type  `json:"-"`
	Comment string `json:",omitempty"`
}

// Represents a type declaration, including all of its constituent fields and methods for structs.
type Type struct {
	File                *File `json:"-"`
	Name                string
	MetaType            string    `json:",omitempty"`
	Methods             []*Method `json:",omitempty"`
	Fields              []*Field  `json:",omitempty"`
	Comment             string    `json:",omitempty"`
	Source              string    `json:",omitempty"`
	HasUnexportedFields bool      `json:",omitempty"`
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
	Imports         []Import `json:",omitempty"`
	MainFunction    bool     `json:",omitempty"`
	Size            int64
	LineCount       int
	SourceLineCount int
	FunctionCount   int
	TypeCount       int
	ConstantCount   int
	VariableCount   int
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
						Name:    vspec.Names[0].String(),
						Type:    astTypeToString(vspec.Type),
						Comment: formatAstComment(gen.Doc),
					}

					for _, val := range vspec.Values {
						if expr := strings.TrimSpace(mustAstNodeToString(val)); len(expr) <= MaxExpressionSnippetLength {
							value.Expression = expr

							// I'm certain there's a better way to do this in go/<somewhere>, but am not up for finding it now.
							// TODO: make this more-better
							if stringutil.IsSurroundedBy(expr, "`", "`") {
								value.Value = stringutil.Unwrap(expr, "`", "`")
							} else if stringutil.IsSurroundedBy(expr, `"`, `"`) {
								value.Value = stringutil.Unwrap(expr, `"`, `"`)
							}
						}

						break
					}

					switch gen.Tok {
					case token.CONST:
						value.Immutable = true
						self.Package.Constants = append(self.Package.Constants, value)
						self.ConstantCount += 1
					case token.VAR:
						self.Package.Variables = append(self.Package.Variables, value)
						self.VariableCount += 1
					}

				case *ast.TypeSpec: // type declarations
					self.appendTypeDecl(gen, spec.(*ast.TypeSpec))
				}
			}
		default:
			log.Debugf("%s: unhandled: %T", self.Name, decl)
		}
	}

	return nil
}

func (self *File) appendFuncDecl(fn *ast.FuncDecl) {
	method := new(Method)
	method.File = self
	method.Name = fn.Name.Name
	method.Comment = formatAstComment(fn.Doc)

	if method.Name == `main` {
		self.MainFunction = true
	}

	if ast.IsExported(method.Name) {
		var constructorTypeName string

		// TODO: make this anon function a package-level exported function
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
			for i, res := range fn.Type.Results.List {
				var name string

				if len(res.Names) > 0 {
					name = res.Names[0].String()
				}

				retType := astTypeToString(res.Type)

				method.Returns = append(method.Returns, Arg{
					Name: name,
					Type: retType,
				})

				if t := self.describesDeclaredType(retType); i == 0 && t != `` {
					constructorTypeName = t
				}
			}
		}

		// no receiver == package-level function
		if fn.Recv == nil {
			method.IsPackageLevel = true
			method.Source = base64.StdEncoding.EncodeToString(
				[]byte(mustAstNodeToString(fn.Body)),
			)

			// ...except if it's first return argument type has been declared as a struct
			// in this package.  If so, we put it with that struct's methods
			if typ, ok := self.Package.Types[constructorTypeName]; ok {
				typ.Methods = append(typ.Methods, method)

				// NOTE: this still counts as a package-level function even through we're putting it in a type
				self.FunctionCount += 1
			} else if strings.HasPrefix(method.Name, `Test`) {
				self.Package.Tests = append(self.Package.Tests, method)
			} else if strings.HasPrefix(method.Name, `Example`) {
				pair := strings.TrimPrefix(method.Name, `Example`)
				method.For, method.Label = stringutil.SplitPair(pair, `_`)
				method.Label = stringutil.Camelize(method.Label)

				self.Package.Examples = append(self.Package.Examples, method)
			} else {
				self.Package.Functions = append(self.Package.Functions, method)
				self.FunctionCount += 1
			}

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
					var parent *Type = self.Package.Types[recvName]

					if parent == nil {
						parent = new(Type)
						parent.Name = recvName
						parent.MetaType = `struct`
					}

					parent.Methods = append(parent.Methods, method)
					self.FunctionCount += 1

					sort.Slice(parent.Methods, func(i int, j int) bool {
						return parent.Methods[i].Name < parent.Methods[j].Name
					})

					self.Package.Types[recvName] = parent
				} else {
					return
				}
			}
		}
	}
}

func (self *File) appendTypeDecl(meta *ast.GenDecl, tspec *ast.TypeSpec) {
	if name := tspec.Name.Name; ast.IsExported(name) {
		var typ = new(Type)

		if s, ok := self.Package.Types[name]; ok {
			typ = s
		} else {
			typ = new(Type)
		}

		typ.Name = name
		src := mustAstNodeToString(meta)

		if strings.Contains(src, CommentExportedFields) {
			src = strings.ReplaceAll(src, CommentExportedFields, ``)
			typ.HasUnexportedFields = true
		}

		typ.Source = base64.StdEncoding.EncodeToString([]byte(src))

		// structs have an extra bit of business
		switch tspec.Type.(type) {
		case *ast.StructType:
			strct := tspec.Type.(*ast.StructType)
			typ.MetaType = `struct`
			typ.Comment = formatAstComment(meta.Doc)
			typ.Fields = make([]*Field, 0)

			for _, field := range strct.Fields.List {
				if len(field.Names) > 0 {
					if fieldName := field.Names[0].String(); ast.IsExported(fieldName) {
						typ.Fields = append(typ.Fields, &Field{
							Name:    fieldName,
							Type:    astTypeToString(field.Type),
							Comment: formatAstComment(field.Doc),
						})
					}
				}
			}
		case *ast.Ident:
			typ.MetaType = typeutil.String(tspec.Type)
		default:
			// log.Dump(`typedecl: unhandled metatype: `, tspec.Type)
		}

		self.Package.Types[name] = typ
		self.TypeCount += 1
	}
}

func (self *File) describesDeclaredType(typestr string) string {
	for typeName, _ := range self.Package.Types {
		typestr = strings.TrimPrefix(typestr, `*`)

		if typeName == typestr {
			return typeName
		}
	}

	return ``
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
	case *ast.SelectorExpr:
		sel := typ.(*ast.SelectorExpr)
		return typeutil.String(sel.X) + `.` + sel.Sel.String()
	case *ast.InterfaceType:
		return `interface{}`
	case *ast.Ellipsis:
		return `...` + astTypeToString(typ.(*ast.Ellipsis).Elt)
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

// func astFileExamples(file *ast.File) (examples []Example) {
// 	for _, ex := range doc.Examples(file) {
// 		examples = append(examples, Example{
// 			Name:    ex.Name,
// 			Comment: ex.Doc,
// 			Output:  ex.Output,
// 		})
// 	}

// 	return
// }

func mustAstNodeToString(node ast.Node) string {
	var buf bytes.Buffer
	var fset = token.NewFileSet()

	if err := format.Node(&buf, fset, node); err == nil {
		return buf.String()
	} else {
		panic(err.Error())
	}
}
