// Copyright 2013 Gary Burd
//
// Licensed under the Apache License, Version 2.0 (the "License"): you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
// WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
// License for the specific language governing permissions and limitations
// under the License.

// Method fingerprint bugs:
// - Embedded interfaces are not expanded.
// - Inline interface methods are not sorted to a canonical order.
// - Array size expressions are not evaluated.
// - Unnecessary use of () are not removed.

package doc

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"go/ast"
	"go/token"
	"sort"
	"strconv"
)

type Method struct {
	Name        string
	Fingerprint Fingerprint
	IsPtr       bool
}

type EmbeddedField struct {
	Name  string
	Path  string
	IsPtr bool
}

type MethodSet struct {
	Methods        []*Method
	EmbeddedFields []*EmbeddedField
	Errors         []string
	IsInterface    bool
}

type Fingerprint [16]byte

func (fp Fingerprint) String() string {
	return hex.EncodeToString(fp[:])
}

type aborted struct{ err error }

func abort(err error) { panic(aborted{err}) }

func handleAbort(err *error) {
	if r := recover(); r != nil {
		if a, ok := r.(aborted); ok {
			*err = a.err
		} else {
			panic(r)
		}
	}
}

type funcDecl struct {
	name  string
	typ   *ast.FuncType
	isPtr bool
}

type fingerprinter struct {
	fset                  *token.FileSet
	pkg                   *ast.Package
	path                  string // import path for this package.
	quotedPath            string // quoted import path for this package.
	buf                   []byte // Collects canonical method declaration.
	exportedInterfaceMode bool
	funcDecls             map[string][]*funcDecl
	visited               map[string]bool
	include               map[string][]Fingerprint

	methodSets map[string]*MethodSet
}

func (p *fingerprinter) nodePath(n ast.Node) (string, error) {
	if n, _ := n.(*ast.Ident); n != nil {
		if obj := n.Obj; obj != nil && obj.Kind == ast.Pkg {
			if spec, _ := obj.Decl.(*ast.ImportSpec); spec != nil {
				return spec.Path.Value, nil
			}
		}
		return "", fmt.Errorf("%s not resolved (%s)", n.Name, p.fset.Position(n.Pos()))
	}
	return "", fmt.Errorf("unexpected %T (%s)", n, p.fset.Position(n.Pos()))
}

func (p *fingerprinter) writeFunc(n *ast.FuncType) {
	p.writeParams(n.Params, true)
	p.writeParams(n.Results, n.Results != nil && n.Results.NumFields() > 1)
}

func (p *fingerprinter) writeParams(list *ast.FieldList, paren bool) {
	var sep bool
	if paren {
		p.buf = append(p.buf, '(')
	}
	if list != nil {
		for _, field := range list.List {
			m := len(field.Names)
			if m == 0 {
				m = 1
			}
			for i := 0; i < m; i++ {
				if sep {
					p.buf = append(p.buf, ',')
				} else {
					sep = true
				}
				p.writeNode(field.Type)
			}
		}
	}
	if paren {
		p.buf = append(p.buf, ')')
	}
}

var anonymousNames = []*ast.Ident{nil}

func (p *fingerprinter) writeStruct(s *ast.StructType) {
	p.buf = append(p.buf, "struct{"...)
	var sep bool
	if s.Fields != nil {
		for _, field := range s.Fields.List {
			names := field.Names
			if len(names) == 0 {
				names = anonymousNames
			}
			for _, name := range names {
				if sep {
					p.buf = append(p.buf, ';')
				} else {
					sep = true
				}
				if name != nil {
					p.buf = append(p.buf, name.Name...)
					p.buf = append(p.buf, ' ')
				}
				p.writeNode(field.Type)
				if field.Tag != nil {
					p.buf = append(p.buf, ' ')
					tag, err := strconv.Unquote(field.Tag.Value)
					if err != nil {
						// canonical quoting
						p.buf = append(p.buf, strconv.Quote(tag)...)
					} else {
						p.buf = append(p.buf, field.Tag.Value...)
					}
				}
			}
		}
	}
	p.buf = append(p.buf, '}')
}

func (p *fingerprinter) writeInterface(s *ast.InterfaceType) {
	p.buf = append(p.buf, "interface{"...)
	var sep bool
	if s.Methods != nil {
		for _, field := range s.Methods.List {
			names := field.Names
			if len(names) == 0 {
				names = anonymousNames
			}
			for _, name := range names {
				if sep {
					p.buf = append(p.buf, ';')
				} else {
					sep = true
				}
				switch n := field.Type.(type) {
				case *ast.Ident:
					p.writeNode(n)
				case *ast.SelectorExpr:
					p.writeNode(n)
				case *ast.FuncType:
					if name != nil {
						p.buf = append(p.buf, name.Name...)
					}
					p.writeFunc(field.Type.(*ast.FuncType))
				default:
					abort(fmt.Errorf("unexpected %T (%s)", n, p.fset.Position(n.Pos())))
				}
			}
		}
	}
	p.buf = append(p.buf, '}')
}

func (p *fingerprinter) writeNode(n ast.Node) {
	switch n := n.(type) {
	case *ast.Ellipsis:
		p.buf = append(p.buf, "..."...)
		p.writeNode(n.Elt)
	case *ast.MapType:
		p.buf = append(p.buf, "map["...)
		p.writeNode(n.Key)
		p.buf = append(p.buf, ']')
		p.writeNode(n.Value)
	case *ast.ArrayType:
		p.buf = append(p.buf, '[')
		if n.Len != nil {
			p.writeNode(n.Len)
		}
		p.buf = append(p.buf, ']')
		p.writeNode(n.Elt)
	case *ast.ChanType:
		if n.Dir == ast.RECV {
			p.buf = append(p.buf, "<-"...)
		}
		p.buf = append(p.buf, "chan"...)
		if n.Dir == ast.SEND {
			p.buf = append(p.buf, "<-"...)
		}
		p.buf = append(p.buf, ' ')
		p.writeNode(n.Value)
	case *ast.ParenExpr:
		p.buf = append(p.buf, '(')
		p.writeNode(n.X)
		p.buf = append(p.buf, ')')
	case *ast.BinaryExpr:
		p.writeNode(n.X)
		p.buf = append(p.buf, n.Op.String()...)
		p.writeNode(n.Y)
	case *ast.BasicLit:
		p.buf = append(p.buf, n.Value...)
	case *ast.StarExpr:
		p.buf = append(p.buf, '*')
		p.writeNode(n.X)
	case *ast.FuncDecl:
		p.buf = append(p.buf, n.Name.Name...)
		p.writeFunc(n.Type)
	case *ast.FuncType:
		p.buf = append(p.buf, "func"...)
		p.writeFunc(n)
	case *ast.InterfaceType:
		p.writeInterface(n)
	case *ast.StructType:
		p.writeStruct(n)
	case *ast.SelectorExpr:
		path, err := p.nodePath(n.X)
		if err != nil {
			abort(err)
		}
		p.buf = append(p.buf, path...)
		p.buf = append(p.buf, '.')
		p.buf = append(p.buf, n.Sel.Name...)
	case *ast.Ident:
		if n.Obj != nil || predeclared[n.Name] != predeclaredType {
			p.buf = append(p.buf, p.quotedPath...)
			p.buf = append(p.buf, '.')
		}
		p.buf = append(p.buf, n.Name...)
	default:
		abort(fmt.Errorf("unexpected %T (%s)", n, p.fset.Position(n.Pos())))
	}
}

func (p *fingerprinter) method(fd *funcDecl) (method *Method, err error) {
	defer handleAbort(&err)
	p.buf = p.buf[:0]
	p.writeFunc(fd.typ)
	h := md5.New()
	h.Write(p.buf)
	method = &Method{Name: fd.name, IsPtr: fd.isPtr}
	h.Sum(method.Fingerprint[:0])
	return
}

type methodByName []*Method

func (p methodByName) Len() int           { return len(p) }
func (p methodByName) Less(i, j int) bool { return p[i].Name < p[j].Name }
func (p methodByName) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }

type embeddedFieldByName []*EmbeddedField

func (p embeddedFieldByName) Len() int           { return len(p) }
func (p embeddedFieldByName) Less(i, j int) bool { return p[i].Name < p[j].Name }
func (p embeddedFieldByName) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }

func (p *fingerprinter) collectFuncDecls() {
	for _, src := range p.pkg.Files {
		for _, decl := range src.Decls {
			decl, ok := decl.(*ast.FuncDecl)
			if !ok || decl.Recv == nil {
				continue
			}
			list := decl.Recv.List
			if len(list) != 1 {
				// not a method
				continue
			}
			var recv string
			var isPtr bool
			switch n := list[0].Type.(type) {
			case *ast.Ident:
				recv = n.Name
			case *ast.StarExpr:
				if n, ok := n.X.(*ast.Ident); ok {
					isPtr = true
					recv = n.Name
				}
			}
			if recv == "" {
				// bad syntax, ignore
				continue
			}
			p.funcDecls[recv] = append(p.funcDecls[recv], &funcDecl{
				name:  decl.Name.Name,
				typ:   decl.Type,
				isPtr: isPtr})
		}
	}
}

func (p *fingerprinter) visitExportedTypes() {
	for name, obj := range p.pkg.Scope.Objects {
		if ast.IsExported(name) || p.path == "builtin" {
			if spec, ok := obj.Decl.(*ast.TypeSpec); ok {
				if _, ok := spec.Type.(*ast.InterfaceType); ok == p.exportedInterfaceMode {
					p.visitType(spec)
				}
			}
		}
	}
}

func (p *fingerprinter) visitType(spec *ast.TypeSpec) bool {
	name := spec.Name.Name
	if p.visited[name] {
		return p.methodSets[name] != nil
	}
	p.visited[name] = true // todo: improve handling of recursive types.

	var (
		funcDecls      = p.funcDecls[name]
		embeddedFields []*EmbeddedField
		isInterface    bool
		errors         []string
	)

	var fields []*ast.Field
	switch n := spec.Type.(type) {
	case *ast.StructType:
		fields = n.Fields.List
	case *ast.InterfaceType:
		fields = n.Methods.List
		isInterface = true
		funcDecls = nil
	}

	for _, field := range fields {
		switch len(field.Names) {
		case 1:
			if isInterface {
				if n, ok := field.Type.(*ast.FuncType); ok {
					funcDecls = append(funcDecls, &funcDecl{name: field.Names[0].Name, typ: n, isPtr: false})
				}
			}
		case 0:
			// Embedded fields and interfaces

			var isPtr bool
			n := field.Type

			if !isInterface {
				if star, ok := n.(*ast.StarExpr); ok {
					isPtr = true
					n = star.X
				}
			}

			switch n := n.(type) {
			case *ast.SelectorExpr:
				path, err := p.nodePath(n.X)
				if err != nil {
					errors = append(errors, err.Error())
				} else {
					embeddedFields = append(embeddedFields, &EmbeddedField{
						Name:  n.Sel.Name,
						Path:  path,
						IsPtr: isPtr})
				}
			case *ast.Ident:
				ef := &EmbeddedField{Name: n.Name, Path: p.path, IsPtr: isPtr}
				if n.Obj == nil {
					if ef.Name == "error" {
						ef.Path = "builtin"
					}
				} else if spec, ok := n.Obj.Decl.(*ast.TypeSpec); ok && !p.visitType(spec) {
					// The embedded type was found and does not contain intereting methods.
					ef = nil
				}
				if ef != nil {
					embeddedFields = append(embeddedFields, ef)
				}
			}
		}
	}

	var methods []*Method
	for _, fd := range funcDecls {
		if ast.IsExported(fd.name) || p.exportedInterfaceMode || p.include[fd.name] != nil {
			method, err := p.method(fd)
			if err != nil {
				errors = append(errors, err.Error())
				continue
			}
			switch {
			case ast.IsExported(fd.name):
				methods = append(methods, method)
			case p.exportedInterfaceMode:
				methods = append(methods, method)
				p.include[fd.name] = append(p.include[fd.name], method.Fingerprint)
			default:
				for _, fp := range p.include[fd.name] {
					if method.Fingerprint == fp {
						methods = append(methods, method)
						break
					}
				}
			}
		}
	}

	if len(methods) == 0 && len(embeddedFields) == 0 && len(errors) == 0 {
		return false
	}

	sort.Sort(methodByName(methods))
	sort.Sort(embeddedFieldByName(embeddedFields))
	p.methodSets[name] = &MethodSet{
		IsInterface:    isInterface,
		Methods:        methods,
		EmbeddedFields: embeddedFields,
		Errors:         errors,
	}
	return true
}

func methodSets(fset *token.FileSet, pkg *ast.Package, importPath string) (map[string]*MethodSet, error) {
	p := fingerprinter{
		fset:       fset,
		pkg:        pkg,
		path:       importPath,
		quotedPath: strconv.Quote(importPath),
		funcDecls:  make(map[string][]*funcDecl),
		visited:    make(map[string]bool),
		include:    make(map[string][]Fingerprint),
		methodSets: make(map[string]*MethodSet),
	}
	p.collectFuncDecls()
	p.exportedInterfaceMode = true
	p.visitExportedTypes()
	p.exportedInterfaceMode = false
	p.visitExportedTypes()
	return p.methodSets, nil
}

/*
    Path, Type, Name
func MethodsForType(importPath string, typ string, resolver()) [][3]string {
    get metods sets for import path
    lookup type in method sets
*/
