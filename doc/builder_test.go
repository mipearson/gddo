// Copyright 2012 Gary Burd
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
	"testing"
)

var badSynopsis = []string{
	"+build !release",
	"COPYRIGHT Jimmy Bob",
	"### Markdown heading",
	"-*- indent-tabs-mode: nil -*-",
	"vim:set ts=2 sw=2 et ai ft=go:",
}

func TestBadSynopsis(t *testing.T) {
	for _, s := range badSynopsis {
		if synopsis(s) != "" {
			t.Errorf(`synopsis(%q) did not return ""`, s)
		}
	}
}

const readme = `
    $ go get github.com/user/repo/pkg1
    [foo](http://gopkgdoc.appspot.com/pkg/github.com/user/repo/pkg2)
    [foo](http://go.pkgdoc.org/github.com/user/repo/pkg3)
    [foo](http://godoc.org/github.com/user/repo/pkg4)
    <http://go.pkgdoc.org/github.com/user/repo/pkg5>
    [foo](http://godoc.org/github.com/user/repo/pkg6#Export)
    'go get example.org/package1' will install package1.
    (http://go.pkgdoc.org/example.org/package2 "Package2's documentation on GoPkgDoc").
    import "example.org/package3"
`

var expectedReferences = []string{
	"github.com/user/repo/pkg1",
	"github.com/user/repo/pkg2",
	"github.com/user/repo/pkg3",
	"github.com/user/repo/pkg4",
	"github.com/user/repo/pkg5",
	"github.com/user/repo/pkg6",
	"example.org/package1",
	"example.org/package2",
	"example.org/package3",
}

func TestReferences(t *testing.T) {
	references := make(map[string]bool)
	addReferences(references, []byte(readme))
	for _, r := range expectedReferences {
		if !references[r] {
			t.Errorf("missing %s", r)
		}
		delete(references, r)
	}
	for r := range references {
		t.Errorf("extra %s", r)
	}
}

const fileImportSrc = `
package foobar
import (
    a "example.com/z"
    "exampel.com/a"
    "example.com/a.go"
    "example.com/go.a"
    "example.com/b"
    "example.com/b-go"
    "example.com/go-b"
    "example.com/goc"
    "example.com/d.go"
)
`

var expectedFileImports = map[string]map[string]string{
	"doc.go": map[string]string{
		"a":   "example.com/z",
		"b":   "example.com/b",
		"c":   "example.com/goc",
		"goc": "example.com/goc",
		"d":   "example.com/d.go",

		".a":   "example.com/go.a",
		"a.go": "example.com/a.go",
		"-b":   "example.com/go-b",
		"b-go": "example.com/b-go",
		"go.a": "example.com/go.a",
		"go-b": "example.com/go-b",
		"d.go": "example.com/d.go",
	},
}

func TestFileImports(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "doc.go", []byte(fileImportSrc), 0)
	if err != nil {
		t.Fatal(err)
	}
	pkg := &ast.Package{Files: map[string]*ast.File{"doc.go": file}}
	actualFileImports := fileImports(pkg)
	if !reflect.DeepEqual(actualFileImports, expectedFileImports) {
		t.Errorf("fileImports=%v, want %v", actualFileImports, expectedFileImports)
	}
}
