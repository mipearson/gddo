// Copyright 2011 Gary Burd
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

package doc

import (
	"go/ast"
	"go/printer"
	"go/scanner"
	"go/token"
	"strconv"
)

const (
	notPredeclared = iota
	predeclaredType
	predeclaredConstant
	predeclaredFunction
)

// predeclared represents the set of all predeclared identifiers.
var predeclared = map[string]int{
	"bool":       predeclaredType,
	"byte":       predeclaredType,
	"complex128": predeclaredType,
	"complex64":  predeclaredType,
	"error":      predeclaredType,
	"float32":    predeclaredType,
	"float64":    predeclaredType,
	"int16":      predeclaredType,
	"int32":      predeclaredType,
	"int64":      predeclaredType,
	"int8":       predeclaredType,
	"int":        predeclaredType,
	"rune":       predeclaredType,
	"string":     predeclaredType,
	"uint16":     predeclaredType,
	"uint32":     predeclaredType,
	"uint64":     predeclaredType,
	"uint8":      predeclaredType,
	"uint":       predeclaredType,
	"uintptr":    predeclaredType,

	"true":  predeclaredConstant,
	"false": predeclaredConstant,
	"iota":  predeclaredConstant,
	"nil":   predeclaredConstant,

	"append":  predeclaredFunction,
	"cap":     predeclaredFunction,
	"close":   predeclaredFunction,
	"complex": predeclaredFunction,
	"copy":    predeclaredFunction,
	"delete":  predeclaredFunction,
	"imag":    predeclaredFunction,
	"len":     predeclaredFunction,
	"make":    predeclaredFunction,
	"new":     predeclaredFunction,
	"panic":   predeclaredFunction,
	"print":   predeclaredFunction,
	"println": predeclaredFunction,
	"real":    predeclaredFunction,
	"recover": predeclaredFunction,
}

type AnnotationKind int32

const (
	// Link to export in package specifed by Paths[PathIndex] with fragment
	// Text[strings.LastIndex(Text[Pos:End], ".")+1:End].
	ExportLinkAnnotation AnnotationKind = iota

	// Anchor with name specified by Text[Pos:End] or typeName + "." +
	// Text[Pos:End] for type declarations.
	AnchorAnnotation

	// Comment.
	CommentAnnotation

	// Link to package specified by Paths[PathIndex].
	PackageLinkAnnotation

	// Link to builtin entity with name Text[Pos:End].
	BuiltinAnnotation
)

type Annotation struct {
	Pos, End  int32
	Kind      AnnotationKind
	PathIndex int32
}

type Code struct {
	Text        string
	Annotations []Annotation
	Paths       []string
}

// annotationVisitor collects annotations.
type annotationVisitor struct {
	annotations []Annotation
	paths       []string
	pathIndex   map[string]int
}

func (v *annotationVisitor) add(kind AnnotationKind, importPath string) {
	pathIndex := -1
	if importPath != "" {
		var ok bool
		pathIndex, ok = v.pathIndex[importPath]
		if !ok {
			pathIndex = len(v.paths)
			v.paths = append(v.paths, importPath)
			v.pathIndex[importPath] = pathIndex
		}
	}
	v.annotations = append(v.annotations, Annotation{Kind: kind, PathIndex: int32(pathIndex)})
}

func (v *annotationVisitor) ignoreName() {
	v.add(-1, "")
}

func (v *annotationVisitor) Visit(n ast.Node) ast.Visitor {
	switch n := n.(type) {
	case *ast.TypeSpec:
		v.ignoreName()
		var list *ast.FieldList
		switch n := n.Type.(type) {
		case *ast.InterfaceType:
			list = n.Methods
		case *ast.StructType:
			list = n.Fields
		}
		if list == nil {
			ast.Walk(v, n.Type)
		} else {
			for _, n := range list.List {
				for _ = range n.Names {
					v.add(AnchorAnnotation, "")
				}
				ast.Walk(v, n.Type)
			}
		}
	case *ast.FuncDecl:
		if n.Recv != nil {
			ast.Walk(v, n.Recv)
		}
		v.ignoreName()
		ast.Walk(v, n.Type)
	case *ast.Field:
		for _ = range n.Names {
			v.ignoreName()
		}
		ast.Walk(v, n.Type)
	case *ast.ValueSpec:
		for _ = range n.Names {
			v.add(AnchorAnnotation, "")
		}
		if n.Type != nil {
			ast.Walk(v, n.Type)
		}
		for _, x := range n.Values {
			ast.Walk(v, x)
		}
	case *ast.Ident:
		switch {
		case n.Obj == nil && predeclared[n.Name] != notPredeclared:
			v.add(BuiltinAnnotation, "")
		case n.Obj != nil && ast.IsExported(n.Name):
			v.add(ExportLinkAnnotation, "")
		default:
			v.ignoreName()
		}
	case *ast.SelectorExpr:
		if x, _ := n.X.(*ast.Ident); x != nil {
			if obj := x.Obj; obj != nil && obj.Kind == ast.Pkg {
				if spec, _ := obj.Decl.(*ast.ImportSpec); spec != nil {
					if path, err := strconv.Unquote(spec.Path.Value); err == nil {
						v.add(PackageLinkAnnotation, path)
						if path == "C" {
							v.ignoreName()
						} else {
							v.add(ExportLinkAnnotation, path)
						}
						return nil
					}
				}
			}
		}
		ast.Walk(v, n.X)
		v.ignoreName()
	default:
		return v
	}
	return nil
}

func printDecl(decl ast.Node, fset *token.FileSet, buf []byte) (Code, []byte) {
	v := &annotationVisitor{pathIndex: make(map[string]int)}
	ast.Walk(v, decl)
	buf = buf[:0]
	err := (&printer.Config{Mode: printer.UseSpaces, Tabwidth: 4}).Fprint(sliceWriter{&buf}, fset, decl)
	if err != nil {
		return Code{Text: err.Error()}, buf
	}

	var annotations []Annotation
	var s scanner.Scanner
	fset = token.NewFileSet()
	file := fset.AddFile("", fset.Base(), len(buf))
	s.Init(file, buf, nil, scanner.ScanComments)
loop:
	for {
		pos, tok, lit := s.Scan()
		switch tok {
		case token.EOF:
			break loop
		case token.COMMENT:
			p := file.Offset(pos)
			e := p + len(lit)
			annotations = append(annotations, Annotation{Kind: CommentAnnotation, Pos: int32(p), End: int32(e)})
		case token.IDENT:
			if len(v.annotations) == 0 {
				// Oops!
				break loop
			}
			annotation := v.annotations[0]
			v.annotations = v.annotations[1:]
			if annotation.Kind == -1 {
				continue
			}
			p := file.Offset(pos)
			e := p + len(lit)
			annotation.Pos = int32(p)
			annotation.End = int32(e)
			if len(annotations) > 0 && annotation.Kind == ExportLinkAnnotation {
				prev := annotations[len(annotations)-1]
				if prev.Kind == PackageLinkAnnotation &&
					prev.PathIndex == annotation.PathIndex &&
					prev.End+1 == annotation.Pos {
					// merge with previous
					annotation.Pos = prev.Pos
					annotations[len(annotations)-1] = annotation
					continue loop
				}
			}
			annotations = append(annotations, annotation)
		}
	}
	return Code{Text: string(buf), Annotations: annotations, Paths: v.paths}, buf
}

func commentAnnotations(src string) []Annotation {
	var annotations []Annotation
	var s scanner.Scanner
	fset := token.NewFileSet()
	file := fset.AddFile("", fset.Base(), len(src))
	s.Init(file, []byte(src), nil, scanner.ScanComments)
	for {
		pos, tok, lit := s.Scan()
		switch tok {
		case token.EOF:
			return annotations
		case token.COMMENT:
			p := file.Offset(pos)
			e := p + len(lit)
			annotations = append(annotations, Annotation{Kind: CommentAnnotation, Pos: int32(p), End: int32(e)})
		}
	}
	return nil
}
