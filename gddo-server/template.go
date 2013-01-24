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

package main

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	godoc "go/doc"
	"go/scanner"
	"go/token"
	htemp "html/template"
	"io"
	"io/ioutil"
	"log"
	"net/url"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"
	ttemp "text/template"
	"time"

	"github.com/garyburd/gopkgdoc/doc"
	"github.com/garyburd/indigo/web"
)

func escapePath(s string) string {
	u := url.URL{Path: s}
	return u.String()
}

var (
	staticMutex sync.RWMutex
	staticHash  = make(map[string]string)
)

func staticFileFn(p string) htemp.URL {
	staticMutex.RLock()
	h, ok := staticHash[p]
	staticMutex.RUnlock()

	if !ok {
		fp := filepath.Join(*staticDir, filepath.FromSlash(p))
		b, err := ioutil.ReadFile(fp)
		if err != nil {
			log.Printf("WARNING could not read static file %s", fp)
			return htemp.URL("/-/static/" + p)
		}

		m := md5.New()
		m.Write(b)
		h = hex.EncodeToString(m.Sum(nil))

		staticMutex.Lock()
		staticHash[p] = h
		staticMutex.Unlock()
	}

	return htemp.URL("/-/static/" + p + "?v=" + h)
}

func mapFn(kvs ...interface{}) (map[string]interface{}, error) {
	if len(kvs)%2 != 0 {
		return nil, errors.New("map requires even number of arguments.")
	}
	m := make(map[string]interface{})
	for i := 0; i < len(kvs); i += 2 {
		s, ok := kvs[i].(string)
		if !ok {
			return nil, errors.New("even args to map must be strings.")
		}
		m[s] = kvs[i+1]
	}
	return m, nil
}

// relativePathFn formats an import path as HTML.
func relativePathFn(path string, parentPath interface{}) string {
	if p, ok := parentPath.(string); ok && p != "" && strings.HasPrefix(path, p) {
		path = path[len(p)+1:]
	}
	return path
}

// importPathFn formats an import with zero width space characters to allow for breaks.
func importPathFn(path string) htemp.HTML {
	path = htemp.HTMLEscapeString(path)
	if len(path) > 45 {
		// Allow long import paths to break following "/"
		path = strings.Replace(path, "/", "/&#8203;", -1)
	}
	return htemp.HTML(path)
}

// relativeTime formats the time t in nanoseconds as a human readable relative
// time.
func relativeTime(t time.Time) string {
	const day = 24 * time.Hour
	d := time.Now().Sub(t)
	switch {
	case d < time.Second:
		return "just now"
	case d < 2*time.Second:
		return "one second ago"
	case d < time.Minute:
		return fmt.Sprintf("%d seconds ago", d/time.Second)
	case d < 2*time.Minute:
		return "one minute ago"
	case d < time.Hour:
		return fmt.Sprintf("%d minutes ago", d/time.Minute)
	case d < 2*time.Hour:
		return "one hour ago"
	case d < day:
		return fmt.Sprintf("%d hours ago", d/time.Hour)
	case d < 2*day:
		return "one day ago"
	}
	return fmt.Sprintf("%d days ago", d/day)
}

var (
	h3Open     = []byte("<h3 ")
	h4Open     = []byte("<h4 ")
	h3Close    = []byte("</h3>")
	h4Close    = []byte("</h4>")
	rfcRE      = regexp.MustCompile(`RFC\s+(\d{3,4})`)
	rfcReplace = []byte(`<a href="http://tools.ietf.org/html/rfc$1">$0</a>`)
)

// commentFn formats a source code comment as HTML.
func commentFn(v string) htemp.HTML {
	var buf bytes.Buffer
	godoc.ToHTML(&buf, v, nil)
	p := buf.Bytes()
	p = bytes.Replace(p, h3Open, h4Open, -1)
	p = bytes.Replace(p, h3Close, h4Close, -1)
	p = rfcRE.ReplaceAll(p, rfcReplace)
	return htemp.HTML(p)
}

// commentTextFn formats a source code comment as text.
func commentTextFn(v string) string {
	const indent = "    "
	var buf bytes.Buffer
	godoc.ToText(&buf, v, indent, "\t", 80-2*len(indent))
	p := buf.Bytes()
	return string(p)
}

type sortByPos []doc.TypeAnnotation

func (p sortByPos) Len() int           { return len(p) }
func (p sortByPos) Less(i, j int) bool { return p[i].Pos < p[j].Pos }
func (p sortByPos) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }

func formatCode(src []byte, annotations []doc.TypeAnnotation) htemp.HTML {

	// Collect comment positions.
	var (
		comments []doc.TypeAnnotation
		s        scanner.Scanner
	)
	fset := token.NewFileSet()
	file := fset.AddFile("", fset.Base(), len(src))
	s.Init(file, src, nil, scanner.ScanComments)
commentLoop:
	for {
		pos, tok, lit := s.Scan()
		switch tok {
		case token.EOF:
			break commentLoop
		case token.COMMENT:
			p := file.Offset(pos)
			comments = append(comments, doc.TypeAnnotation{Pos: p, End: p + len(lit)})
		}
	}

	// Merge type annotations and comments without modifying the caller's slice
	// of annoations.
	switch {
	case len(comments) == 0:
		// nothing to do
	case len(annotations) == 0:
		annotations = comments
	default:
		annotations = append(comments, annotations...)
		sort.Sort(sortByPos(annotations))
	}

	var buf bytes.Buffer
	last := 0
	for _, a := range annotations {
		htemp.HTMLEscape(&buf, src[last:a.Pos])
		if a.Name != "" {
			p := a.ImportPath
			if p != "" {
				p = "/" + p
			}
			buf.WriteString(`<a href="`)
			buf.WriteString(escapePath(p))
			buf.WriteByte('#')
			buf.WriteString(escapePath(a.Name))
			buf.WriteString(`">`)
			htemp.HTMLEscape(&buf, src[a.Pos:a.End])
			buf.WriteString(`</a>`)
		} else {
			buf.WriteString(`<span class="com">`)
			htemp.HTMLEscape(&buf, src[a.Pos:a.End])
			buf.WriteString(`</span>`)
		}
		last = a.End
	}
	htemp.HTMLEscape(&buf, src[last:])
	return htemp.HTML(buf.String())
}

// declFn formats a Decl as HTML.
func declFn(decl doc.Decl) htemp.HTML {
	return formatCode([]byte(decl.Text), decl.Annotations)
}

func exampleFn(s string) htemp.HTML {
	return formatCode([]byte(s), nil)
}

func pageNameFn(pdoc *doc.Package) string {
	_, name := path.Split(pdoc.ImportPath)
	return name
}

type crumb struct {
	Name string
	URL  string
	Sep  bool
}

func breadcrumbsFn(pdoc *doc.Package, templateName string) htemp.HTML {
	if !strings.HasPrefix(pdoc.ImportPath, pdoc.ProjectRoot) {
		return ""
	}
	var buf bytes.Buffer
	i := 0
	j := len(pdoc.ProjectRoot)
	if j == 0 {
		j = strings.IndexRune(pdoc.ImportPath, '/')
		if j < 0 {
			j = len(pdoc.ImportPath)
		}
	}
	for {
		if i != 0 {
			buf.WriteString(`<span class="muted">/</span>`)
		}
		link := j < len(pdoc.ImportPath) || templateName == "imports.html" || templateName == "importers.html"
		if link {
			buf.WriteString(`<a href="/`)
			buf.WriteString(escapePath(pdoc.ImportPath[:j]))
			buf.WriteString(`">`)
		} else {
			buf.WriteString(`<span class="muted">`)
		}
		buf.WriteString(htemp.HTMLEscapeString(pdoc.ImportPath[i:j]))
		if link {
			buf.WriteString("</a>")
		} else {
			buf.WriteString("</span>")
		}
		i = j + 1
		if i >= len(pdoc.ImportPath) {
			break
		}
		j = strings.IndexRune(pdoc.ImportPath[i:], '/')
		if j < 0 {
			j = len(pdoc.ImportPath)
		} else {
			j += i
		}
	}
	return htemp.HTML(buf.String())
}

type texample struct {
	*doc.Example
	Object     interface{}
	Id         string
	InternalId string
}

func appendExample(examples []*texample, object interface{}, n1, n2 string, example *doc.Example) []*texample {
	under := ""
	if n2 != "" {
		under = "_"
	}
	dash := ""
	if example.Name != "" {
		dash = "-"
	}
	examples = append(examples, &texample{
		Object:     object,
		Example:    example,
		Id:         fmt.Sprintf("_example_%s%s%s%s%s", n1, under, n2, dash, example.Name),
		InternalId: fmt.Sprintf("_example-%d", len(examples)),
	})
	return examples
}

func allExamplesFn(pdoc *doc.Package) (examples []*texample) {
	for _, e := range pdoc.Examples {
		examples = appendExample(examples, pdoc, "package", "", e)
	}
	for _, f := range pdoc.Funcs {
		for _, e := range f.Examples {
			examples = appendExample(examples, f, f.Name, "", e)
		}
	}
	for _, t := range pdoc.Types {
		for _, e := range t.Examples {
			examples = appendExample(examples, t, t.Name, "", e)
		}
		for _, f := range t.Funcs {
			for _, e := range f.Examples {
				examples = appendExample(examples, f, f.Name, "", e)
			}
		}
		for _, f := range t.Methods {
			for _, e := range f.Examples {
				examples = appendExample(examples, f, t.Name, f.Name, e)
			}
		}
	}
	return
}

func objectExamplesFn(object interface{}, all []*texample) (result []*texample) {
	for _, e := range all {
		if e.Object == object {
			result = append(result, e)
		}
	}
	return
}

func gaAccountFn() string {
	return secrets.GAAccount
}

var contentTypes = map[string]string{
	".html": "text/html; charset=utf-8",
	".txt":  "text/plain; charset=utf-8",
}

func executeTemplate(resp web.Response, name string, status int, data interface{}) error {
	contentType, ok := contentTypes[path.Ext(name)]
	if !ok {
		contentType = "text/plain; charset=utf-8"
	}
	t := templates[name]
	if t == nil {
		return fmt.Errorf("Template %s not found", name)
	}
	w := resp.Start(status, web.Header{web.HeaderContentType: {contentType}})
	return t.Execute(w, data)
}

var templates = map[string]interface {
	Execute(io.Writer, interface{}) error
}{}

func parseHTMLTemplates(sets [][]string) error {
	for _, set := range sets {
		templateName := set[0]
		t := htemp.New("")
		t.Funcs(htemp.FuncMap{
			"allExamples":       allExamplesFn,
			"breadcrumbs":       breadcrumbsFn,
			"comment":           commentFn,
			"decl":              declFn,
			"equal":             reflect.DeepEqual,
			"example":           exampleFn,
			"gaAccount":         gaAccountFn,
			"importPath":        importPathFn,
			"isFunc":            func(v interface{}) bool { _, ok := v.(*doc.Func); return ok },
			"isPackage":         func(v interface{}) bool { _, ok := v.(*doc.Package); return ok },
			"isType":            func(v interface{}) bool { _, ok := v.(*doc.Type); return ok },
			"isValidImportPath": doc.IsValidPath,
			"map":               mapFn,
			"objectExamples":    objectExamplesFn,
			"pageName":          pageNameFn,
			"relativePath":      relativePathFn,
			"relativeTime":      relativeTime,
			"staticFile":        staticFileFn,
			"templateName":      func() string { return templateName },
		})
		var files []string
		for _, n := range set {
			files = append(files, filepath.Join(*templateDir, n))
		}
		if _, err := t.ParseFiles(files...); err != nil {
			return err
		}
		t = t.Lookup("ROOT")
		if t == nil {
			return fmt.Errorf("ROOT template not found in %v", files)
		}
		templates[templateName] = t
	}
	return nil
}

func parseTextTemplates(sets [][]string) error {
	for _, set := range sets {
		templateName := set[0]
		t := ttemp.New("")
		t.Funcs(ttemp.FuncMap{
			"comment": commentTextFn,
		})
		var files []string
		for _, n := range set {
			files = append(files, filepath.Join(*templateDir, n))
		}
		if _, err := t.ParseFiles(files...); err != nil {
			return err
		}
		t = t.Lookup("ROOT")
		if t == nil {
			return fmt.Errorf("ROOT template not found in %v", files)
		}
		templates[templateName] = t
	}
	return nil
}
