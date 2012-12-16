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
	"encoding/json"
	"errors"
	"fmt"
	godoc "go/doc"
	"go/scanner"
	"go/token"
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
	"text/template"
	"time"

	"github.com/garyburd/gopkgdoc/doc"
	"github.com/garyburd/indigo/web"
)

var (
	staticMutex sync.RWMutex
	staticHash  = make(map[string]string)
)

func staticFileFn(p string) string {
	staticMutex.RLock()
	h, ok := staticHash[p]
	staticMutex.RUnlock()

	if !ok {
		fp := filepath.Join(*staticDir, filepath.FromSlash(p))
		b, err := ioutil.ReadFile(fp)
		if err != nil {
			log.Printf("WARNING could not read static file %s", fp)
			return "/-/static/" + p
		}

		m := md5.New()
		m.Write(b)
		h = hex.EncodeToString(m.Sum(nil))

		staticMutex.Lock()
		staticHash[p] = h
		staticMutex.Unlock()
	}

	return "/-/static/" + p + "?v=" + h
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
	return urlFn(path)
}

// importPathFn formats an import with zero width space characters to allow for breaks.
func importPathFn(path string) string {
	path = template.HTMLEscapeString(path)
	if len(path) > 45 {
		// Allow long import paths to break following "/"
		path = strings.Replace(path, "/", "/&#8203;", -1)
	}
	return path
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
func commentFn(v string) string {
	var buf bytes.Buffer
	godoc.ToHTML(&buf, v, nil)
	p := buf.Bytes()
	p = bytes.Replace(p, h3Open, h4Open, -1)
	p = bytes.Replace(p, h3Close, h4Close, -1)
	p = rfcRE.ReplaceAll(p, rfcReplace)
	return string(p)
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

func formatCode(src []byte, annotations []doc.TypeAnnotation) string {

	// Collect comment positions in type annotation with Name = ""
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
		template.HTMLEscape(&buf, src[last:a.Pos])
		if a.Name != "" {
			p := a.ImportPath
			if p != "" {
				p = "/" + p
			}
			buf.WriteString(`<a href="`)
			buf.WriteString(urlFn(p))
			buf.WriteByte('#')
			buf.WriteString(urlFn(a.Name))
			buf.WriteString(`">`)
			template.HTMLEscape(&buf, src[a.Pos:a.End])
			buf.WriteString(`</a>`)
		} else {
			buf.WriteString(`<span class="com">`)
			template.HTMLEscape(&buf, src[a.Pos:a.End])
			buf.WriteString(`</span>`)
		}
		last = a.End
	}
	template.HTMLEscape(&buf, src[last:])
	return buf.String()
}

// declFn formats a Decl as HTML.
func declFn(decl doc.Decl) string {
	return formatCode([]byte(decl.Text), decl.Annotations)
}

func pageNameFn(pdoc *doc.Package) string {
	_, name := path.Split(pdoc.ImportPath)
	return template.HTMLEscapeString(name)
}

type crumb struct {
	Name string
	URL  string
	Sep  bool
}

func breadcrumbsFn(pdoc *doc.Package, page string) string {
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
		link := j < len(pdoc.ImportPath) || page == "imp"
		if link {
			buf.WriteString(`<a href="/`)
			buf.WriteString(urlFn(pdoc.ImportPath[:j]))
			buf.WriteString(`">`)
		} else {
			buf.WriteString(`<span class="muted">`)
		}
		buf.WriteString(template.HTMLEscapeString(pdoc.ImportPath[i:j]))
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
	return buf.String()
}

func urlFn(path string) string {
	u := url.URL{Path: path}
	return u.String()
}

func jsonFn(v interface{}) (string, error) {
	p, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(p), nil
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

func exampleFn(s string) string {
	return formatCode([]byte(s), nil)
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

	w := resp.Start(status, web.Header{web.HeaderContentType: {contentType}})
	return templateSet.ExecuteTemplate(w, name, data)
}

var templateSet *template.Template

func parseTemplates(dir string) (*template.Template, error) {
	// Is there a better way to call ParseGlob with application specified
	// funcs? The dummy template thing is gross.
	set, err := template.New("__dummy__").Parse(`{{define "__dummy__"}}{{end}}`)
	if err != nil {
		return nil, err
	}
	set.Funcs(template.FuncMap{
		"json":              jsonFn,
		"staticFile":        staticFileFn,
		"comment":           commentFn,
		"commentText":       commentTextFn,
		"decl":              declFn,
		"equal":             reflect.DeepEqual,
		"map":               mapFn,
		"breadcrumbs":       breadcrumbsFn,
		"pageName":          pageNameFn,
		"relativePath":      relativePathFn,
		"relativeTime":      relativeTime,
		"importPath":        importPathFn,
		"url":               urlFn,
		"objectExamples":    objectExamplesFn,
		"allExamples":       allExamplesFn,
		"example":           exampleFn,
		"gaAccount":         gaAccountFn,
		"isValidImportPath": doc.IsValidPath,
		"isType":            func(v interface{}) bool { _, ok := v.(*doc.Type); return ok },
		"isPackage":         func(v interface{}) bool { _, ok := v.(*doc.Package); return ok },
		"isFunc":            func(v interface{}) bool { _, ok := v.(*doc.Func); return ok },
	})
	_, err = set.ParseGlob(filepath.Join(dir, "*.html"))
	if err != nil {
		return nil, err
	}
	return set.ParseGlob(filepath.Join(dir, "*.txt"))
}
