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

// Method signature bugs:
// - Embedded interfaces are not expanded.
// - Interface methods are not sorted to a canonical order.
// - Array size expressions are not evaluated.
// - Unnecessary use of () are not removed.
// - It is assumed that predeclared types are not shadowed.

package doc

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"go/ast"
	"strconv"
)

var predeclaredTypes = map[string]bool{
	"bool":       true,
	"byte":       true,
	"complex64":  true,
	"complex128": true,
	"error":      true,
	"float32":    true,
	"float64":    true,
	"int":        true,
	"int8":       true,
	"int16":      true,
	"int32":      true,
	"int64":      true,
	"rune":       true,
	"string":     true,
	"uint":       true,
	"uint8":      true,
	"uint16":     true,
	"uint32":     true,
	"uint64":     true,
	"uintptr":    true,
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

const (
	embeddedMask = 1 << iota
	exportedMask
	allMask
)

// MethodSignature represents the name, parameter types and result types of a
// method.
type MethodSignature [16]byte

func (ms MethodSignature) String() string            { return hex.EncodeToString(ms[:]) }
func (ms MethodSignature) IsEmbeddedInterface() bool { return ms[0]&embeddedMask != 0 }
func (ms MethodSignature) IsExported() bool          { return ms[0]&exportedMask != 0 }

// methodWriter writes a canonical representation of a method to its buffer.
type methodWriter struct {
	buf         []byte
	path        string
	importPaths map[string]string
}

func (w *methodWriter) writeFunc(name string, n *ast.FuncType) {
	w.buf = append(w.buf, name...)
	w.writeParams(n.Params, true)
	w.writeParams(n.Results, n.Results != nil && n.Results.NumFields() > 1)
}

func (w *methodWriter) writeParams(list *ast.FieldList, paren bool) {
	var sep bool
	if paren {
		w.buf = append(w.buf, '(')
	}
	if list != nil {
		for _, field := range list.List {
			m := len(field.Names)
			if m == 0 {
				m = 1
			}
			for i := 0; i < m; i++ {
				if sep {
					w.buf = append(w.buf, ',')
				} else {
					sep = true
				}
				w.writeNode(field.Type)
			}
		}
	}
	if paren {
		w.buf = append(w.buf, ')')
	}
}

var anonymousNames = []*ast.Ident{nil}

func (w *methodWriter) writeStruct(s *ast.StructType) {
	w.buf = append(w.buf, "struct{"...)
	var sep bool
	if s.Fields != nil {
		for _, field := range s.Fields.List {
			names := field.Names
			if len(names) == 0 {
				names = anonymousNames
			}
			for _, name := range names {
				if sep {
					w.buf = append(w.buf, ';')
				} else {
					sep = true
				}
				if name != nil {
					w.buf = append(w.buf, name.Name...)
					w.buf = append(w.buf, ' ')
				}
				w.writeNode(field.Type)
				if field.Tag != nil {
					tag, err := strconv.Unquote(field.Tag.Value)
					if err != nil {
						abort(err)
					}
					w.buf = append(w.buf, ' ')
					w.buf = append(w.buf, strconv.Quote(tag)...)
				}
			}
		}
	}
	w.buf = append(w.buf, '}')
}

func (w *methodWriter) writeInterface(s *ast.InterfaceType) {
	w.buf = append(w.buf, "interface{"...)
	var sep bool
	if s.Methods != nil {
		for _, field := range s.Methods.List {
			names := field.Names
			if len(names) == 0 {
				names = anonymousNames
			}
			for _, name := range names {
				if sep {
					w.buf = append(w.buf, ';')
				} else {
					sep = true
				}
				switch n := field.Type.(type) {
				case *ast.Ident:
					w.writeNode(n)
				case *ast.SelectorExpr:
					w.writeNode(n)
				case *ast.FuncType:
					w.writeFunc(name.Name, field.Type.(*ast.FuncType))
				default:
					abort(fmt.Errorf("Unexpected %T in InterfaceType", n))
				}
			}
		}
	}
	w.buf = append(w.buf, '}')
}

func (w *methodWriter) writeNode(n ast.Node) {
	switch n := n.(type) {
	case *ast.Ellipsis:
		w.buf = append(w.buf, "..."...)
		w.writeNode(n.Elt)
	case *ast.MapType:
		w.buf = append(w.buf, "map["...)
		w.writeNode(n.Key)
		w.buf = append(w.buf, ']')
		w.writeNode(n.Value)
	case *ast.ArrayType:
		w.buf = append(w.buf, '[')
		if n.Len != nil {
			w.writeNode(n.Len)
		}
		w.buf = append(w.buf, ']')
		w.writeNode(n.Elt)
	case *ast.ChanType:
		if n.Dir == ast.RECV {
			w.buf = append(w.buf, "<-"...)
		}
		w.buf = append(w.buf, "chan"...)
		if n.Dir == ast.SEND {
			w.buf = append(w.buf, "<-"...)
		}
		w.buf = append(w.buf, ' ')
		w.writeNode(n.Value)
	case *ast.ParenExpr:
		w.buf = append(w.buf, '(')
		w.writeNode(n.X)
		w.buf = append(w.buf, ')')
	case *ast.BinaryExpr:
		w.writeNode(n.X)
		w.buf = append(w.buf, n.Op.String()...)
		w.writeNode(n.Y)
	case *ast.BasicLit:
		w.buf = append(w.buf, n.Value...)
	case *ast.StarExpr:
		w.buf = append(w.buf, '*')
		w.writeNode(n.X)
	case *ast.FuncDecl:
		w.writeFunc(n.Name.Name, n.Type)
	case *ast.FuncType:
		w.writeFunc("func", n)
	case *ast.InterfaceType:
		w.writeInterface(n)
	case *ast.StructType:
		w.writeStruct(n)
	case *ast.SelectorExpr:
		x, ok := n.X.(*ast.Ident)
		if !ok {
			abort(fmt.Errorf("Unxpected %T in SelectorExpr", n.X))
		}
		if n, ok := w.importPaths[x.Name]; ok {
			w.buf = append(w.buf, n...)
		} else {
			w.buf = append(w.buf, x.Name...)
		}
		w.buf = append(w.buf, '#')
		w.buf = append(w.buf, n.Sel.Name...)
	case *ast.Ident:
		if !predeclaredTypes[n.Name] {
			w.buf = append(w.buf, w.path...)
			w.buf = append(w.buf, '#')
		}
		w.buf = append(w.buf, n.Name...)
	default:
		abort(fmt.Errorf("Unexpected %T in method declaration", n))
	}
}

func canonicalMethodDecl(name string, n *ast.FuncType, path string, importPaths map[string]string, buf []byte) (b []byte, err error) {
	defer handleAbort(&err)
	mw := methodWriter{buf[:0], path, importPaths}
	mw.writeFunc(name, n)
	return mw.buf, err
}

func hash(p []byte, exported, embedded bool) MethodSignature {
	h := md5.New()
	h.Write(p)
	var sig MethodSignature
	h.Sum(sig[:])
	sig[0] &^= (allMask - 1)
	if exported {
		sig[0] |= exportedMask
	}
	if embedded {
		sig[0] |= embeddedMask
	}
	return sig
}

func interfaceSignatures(n *ast.InterfaceType, path string, importPaths map[string]string, buf []byte) ([]MethodSignature, []byte, error) {
	if n.Methods == nil {
		return nil, buf, nil
	}
	var sigs []MethodSignature
	for _, field := range n.Methods.List {
		names := field.Names
		if len(names) == 0 {
			names = anonymousNames
		}
		for _, name := range names {
			switch n := field.Type.(type) {
			case *ast.Ident:
				switch n.Name {
				case "error":
					sigs = append(sigs, hash([]byte(`Error()string`), true, false))
				default:
					sigs = append(sigs, hash([]byte(path+"#"+n.Name), ast.IsExported(n.Name), true))
				}
			case *ast.SelectorExpr:
				x, ok := n.X.(*ast.Ident)
				if !ok {
					return nil, nil, fmt.Errorf("Unxpected %T in SelectorExpr", n.X)
				}
				s := x.Name
				if n, ok := importPaths[s]; ok {
					s = n
				}
				s = s + "#" + n.Sel.Name
				sigs = append(sigs, hash([]byte(s), true, true))
			case *ast.FuncType:
				var err error
				buf, err = canonicalMethodDecl(name.Name, n, path, importPaths, buf)
				if err != nil {
					return nil, nil, err
				}
				sigs = append(sigs, hash(buf, ast.IsExported(name.Name), false))
			default:
				return nil, nil, fmt.Errorf("Unexpected %T in InterfaceType", n)
			}
		}
	}
	return sigs, buf, nil
}
