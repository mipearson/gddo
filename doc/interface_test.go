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
	"testing"
)

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
		`BinOp([2+code.google.com/p/project#Const]byte)`,
	},
	{
		`Ptr(c *Config) (d *Config, err error)`,
		`Ptr(*github.com/owner/repo#Config)(*github.com/owner/repo#Config,error)`,
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
		`Interface(interface{io#Reader},interface{Hello()string})`,
	},
	{
		`Struct(a struct {
            pkg.Config
            Section
            a int
            b int "tag"
        })`,
		`Struct(struct{code.google.com/p/project#Config;github.com/owner/repo#Section;a int;b int "tag"})`,
	},
	{
		`Func(functions ...func(A)int) func(B)(int)`,
		`Func(...func(github.com/owner/repo#A)int)func(github.com/owner/repo#B)int`,
	},
	{`Error() string`,
		`Error()string`,
	},
}

var methodSigImportPaths = map[string]string{
	"pkg": "code.google.com/p/project",
}

func TestMethodSig(t *testing.T) {
	var buf []byte
	for _, s := range methodSigTests {
		buf = buf[:0]
		buf = append(buf, "package foo\nfunc "...)
		buf = append(buf, s.src...)
		file, err := parser.ParseFile(token.NewFileSet(), "", buf, 0)
		if err != nil {
			t.Fatalf("parse(%q) -> %v", s.src, err)
		}
		d := file.Decls[0].(*ast.FuncDecl)
		buf, _ = canonicalMethodDecl(d.Name.Name, d.Type, "github.com/owner/repo", methodSigImportPaths, buf)
		if string(buf) != s.expected {
			t.Errorf("canonical(%q) = \n     %q,\nwant %q", s.src, string(buf), s.expected)
		}
	}
}
