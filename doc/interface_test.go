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

package doc

import (
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
)

func testImporter(imports map[string]*ast.Object, path string) (*ast.Object, error) {
	pkg := imports[path]
	if pkg == nil {
		name := path[strings.LastIndex(path, "/")+1:]
		pkg = ast.NewObj(ast.Pkg, name)
		pkg.Data = ast.NewScope(nil)
		imports[path] = pkg
	}
	return pkg, nil
}

func parsePackage(src ...string) (*ast.Package, error) {
	var buf []byte
	for _, s := range src {
		buf = append(buf, s...)
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "file.go", buf, 0)
	if err != nil {
		return nil, err
	}
	pkg, _ := ast.NewPackage(fset, map[string]*ast.File{"file.go": file}, testImporter, nil)
	return pkg, nil
}

var methodSigTests = []struct {
	src, expected string
}{
	{
		`Ellipsis(args ...interface{}) error`,
		`Ellipsis(...interface{})error`,
	},
	{
		`SliceArrayMap(a []byte, b [3]int, c map[string]interface{})`,
		`SliceArrayMap([]byte,[3]int,map[string]interface{})`,
	},
	{
		`BinOp(a [2+pkg.Const]byte)`,
		`BinOp([2+"code.google.com/p/pkg".Const]byte)`,
	},
	{
		`Ptr(c *Config) (d *Config, err error)`,
		`Ptr(*"github.com/owner/repo".Config)(*"github.com/owner/repo".Config,error)`,
	},
	{
		`Chan(a chan int, b <-chan int, c chan<- int)`,
		`Chan(chan int,<-chan int,chan<- int)`,
	},
	{
		`Interface(a interface {
            io.Reader 
        }, b interface {
            Hello() string 
        })`,
		`Interface(interface{"io".Reader},interface{Hello()string})`,
	},
	{
		`Struct(a struct {
            pkg.Config
            Section
            a int
            b int "tag"
        })`,
		`Struct(struct{"code.google.com/p/pkg".Config;"github.com/owner/repo".Section;a int;b int "tag"})`,
	},
	{
		`Func(functions ...func(A)int) func(B)(int)`,
		`Func(...func("github.com/owner/repo".A)int)func("github.com/owner/repo".B)int`,
	},
	{`Error() string`,
		`Error()string`,
	},
}

const methodPrefix = `
package foo
import (
    "io"
    "code.google.com/p/pkg"
)
func `

func TestMethodSig(t *testing.T) {
	for _, s := range methodSigTests {
		pkg, err := parsePackage(methodPrefix, s.src)
		if err != nil {
			t.Errorf("parse(%q) -> %v", s.src, err)
			continue
		}
		file := pkg.Files["file.go"]
		decl := file.Decls[len(file.Decls)-1].(*ast.FuncDecl)
		w := methodWriter{path: strconv.Quote("github.com/owner/repo")}
		w.writeCanonicalMethodDecl(decl.Name.Name, decl.Type)
		if string(w.buf) != s.expected {
			t.Errorf("canonical(%q) = \n     %q,\nwant %q", s.src, string(w.buf), s.expected)
		}
	}
}

const interfacePrefix = `
package foo
import (
    "io"
    "code.google.com/p/pkg"
)
type X interface {
`

const interfaceSuffix = `
}`

var interfaceSigTests = []struct {
	src      string
	expected []MethodSignature
}{
	{
		"Ellipsis(args ...interface{}) error",
		[]MethodSignature{makeSignature([]byte(`Ellipsis(...interface{})error`), true, false)},
	},
	{
		"error",
		[]MethodSignature{makeSignature([]byte(`Error()string`), true, false)},
	},
	{
		"Foo()\nio.Writer",
		[]MethodSignature{
			makeSignature([]byte(`"io".Writer`), true, true),
			makeSignature([]byte(`Foo()`), true, false),
		},
	},
}

func TestInterfaceSigs(t *testing.T) {
	for _, s := range interfaceSigTests {
		pkg, err := parsePackage(interfacePrefix, s.src, interfaceSuffix)
		if err != nil {
			t.Errorf("parse(%q) -> %v", s.src, err)
			continue
		}
		w := methodWriter{path: strconv.Quote("github.com/owner/repo")}
		isigs, err := w.interfaceSignatures(pkg)
		if err != nil {
			t.Errorf("interfaceSigs(%q) -> %v", s.src, err)
			continue
		}
		sort.Sort(byMethodSignature(s.expected))
		if !reflect.DeepEqual(isigs["X"], s.expected) {
			t.Errorf("interfaceSigs(%q) = \n     %v,\nwant %v", s.src, isigs["X"], s.expected)
		}
	}
}
