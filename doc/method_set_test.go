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

func parsePackage(src ...string) (*token.FileSet, *ast.Package, error) {
	var buf []byte
	for _, s := range src {
		buf = append(buf, s...)
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "file.go", buf, 0)
	if err != nil {
		return nil, nil, err
	}
	pkg, _ := ast.NewPackage(fset, map[string]*ast.File{"file.go": file}, testImporter, nil)
	return fset, pkg, nil
}

var methodFingerprintTests = []struct {
	src, out string
}{
	{
		`(args ...interface{}) error`,
		`(...interface{})error`,
	},
	{
		`(a []byte, b [3]int, c map[string]interface{})`,
		`([]byte,[3]int,map[string]interface{})`,
	},
	{
		`(a [2+pkg.Const]byte)`,
		`([2+"code.google.com/p/pkg".Const]byte)`,
	},
	{
		`(c *Config) (d *Config, err error)`,
		`(*"github.com/owner/repo".Config)(*"github.com/owner/repo".Config,error)`,
	},
	{
		`(a chan int, b <-chan int, c chan<- int)`,
		`(chan int,<-chan int,chan<- int)`,
	},
	{
		`(a interface {
            io.Reader 
        }, b interface {
            Hello() string 
        })`,
		`(interface{"io".Reader},interface{Hello()string})`,
	},
	{
		`(a struct {
            pkg.Config
            Section
            a int
            b int "tag"
        })`,
		`(struct{"code.google.com/p/pkg".Config;"github.com/owner/repo".Section;a int;b int "tag"})`,
	},
	{
		`(a struct {
            *Section
        })`,
		`(struct{*"github.com/owner/repo".Section})`,
	},
	{
		`(functions ...func(A)int) func(B)(int)`,
		`(...func("github.com/owner/repo".A)int)func("github.com/owner/repo".B)int`,
	},
	{
		`() string`,
		`()string`,
	},
}

const methodPrefix = `
package foo
import (
    "io"
    "code.google.com/p/pkg"
)
func Exmaple`

func TestMethodFingerprints(t *testing.T) {
	for _, s := range methodFingerprintTests {
		fset, pkg, err := parsePackage(methodPrefix, s.src)
		if err != nil {
			t.Errorf("parse(%q) -> %v", s.src, err)
			continue
		}
		file := pkg.Files["file.go"]
		decl := file.Decls[len(file.Decls)-1].(*ast.FuncDecl)
		p := fingerprinter{fset: fset, path: "cithub.com/owner/repo", quotedPath: strconv.Quote("github.com/owner/repo")}
		p.writeFunc(decl.Type)
		if string(p.buf) != s.out {
			t.Errorf("writeFunc(%q) = \n     %q,\nwant %q", s.src, string(p.buf), s.out)
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

var interfaceFingerprintTests = []struct {
	src     string
	embeds  []string
	methods []string
}{
	{
		"Ellipsis(args ...interface{}) error",
		nil,
		[]string{`Ellipsis(...interface{})error`},
	},
	{
		"error",
		nil,
		[]string{`Error()string`},
	},
	{
		"Foo()\nio.Writer",
		[]string{"Writer io"},
		[]string{`Foo()`},
	},
}

/*
func TestInterfaceFingerprints(t *testing.T) {
	for _, s := range interfaceFingerprintTests {
		pkg, err := parsePackage(interfacePrefix, s.src, interfaceSuffix)
		if err != nil {
			t.Errorf("parse(%q) -> %v", s.src, err)
			continue
		}
		w := fingerprinter{path: strconv.Quote("github.com/owner/repo")}
		if err := w.collectInterfaceFingerprints(pkg); err != nil {
			t.Errorf("interfaceFingerprints(%q) -> %v", s.src, err)
			continue
		}
		sort.Sort(byMethodFingerprint(s.methodSet))
		if !reflect.DeepEqual(w.fingerprints["X"], s.methodSet) {
			t.Errorf("interfaceFingerprints(%q) = \n     %v,\nwant %v", s.src, w.fingerprints["X"], s.methodSet)
		}
	}
}

*/
