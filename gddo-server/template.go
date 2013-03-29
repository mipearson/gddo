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
	htemp "html/template"
	"io"
	"io/ioutil"
	"log"
	"net/url"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"sync"
	ttemp "text/template"
	"time"

	"code.google.com/p/go.talks/pkg/present"

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
		fp := filepath.Join(*baseDir, "static", filepath.FromSlash(p))
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

var period = []byte{'.'}

func codeFn(c doc.Code) htemp.HTML {
	var buf bytes.Buffer
	last := 0
	src := []byte(c.Text)
	for _, a := range c.Annotations {
		htemp.HTMLEscape(&buf, src[last:a.Pos])
		switch a.Kind {
		case doc.PackageLinkAnnotation:
			p := "/" + a.ImportPath
			buf.WriteString(`<a href="`)
			buf.WriteString(escapePath(p))
			buf.WriteString(`">`)
			htemp.HTMLEscape(&buf, src[a.Pos:a.End])
			buf.WriteString(`</a>`)
		case doc.ExportLinkAnnotation, doc.BuiltinAnnotation:
			p := a.ImportPath
			if a.Kind == doc.BuiltinAnnotation {
				p = "/builtin"
			} else if p != "" {
				p = "/" + p
			}
			n := src[a.Pos:a.End]
			n = n[bytes.LastIndex(n, period)+1:]
			buf.WriteString(`<a href="`)
			buf.WriteString(escapePath(p))
			buf.WriteByte('#')
			buf.WriteString(escapePath(string(n)))
			buf.WriteString(`">`)
			htemp.HTMLEscape(&buf, src[a.Pos:a.End])
			buf.WriteString(`</a>`)
		case doc.CommentAnnotation:
			buf.WriteString(`<span class="com">`)
			htemp.HTMLEscape(&buf, src[a.Pos:a.End])
			buf.WriteString(`</span>`)
		case doc.AnchorAnnotation:
			buf.WriteString(`<span id="`)
			htemp.HTMLEscape(&buf, src[a.Pos:a.End])
			buf.WriteString(`">`)
			htemp.HTMLEscape(&buf, src[a.Pos:a.End])
			buf.WriteString(`</span>`)
		default:
			htemp.HTMLEscape(&buf, src[a.Pos:a.End])
		}
		last = int(a.End)
	}
	htemp.HTMLEscape(&buf, src[last:])
	return htemp.HTML(buf.String())
}

func pageNameFn(pdoc *doc.Package) string {
	_, name := path.Split(pdoc.ImportPath)
	return name
}

func hasExamplesFn(pdoc *doc.Package) bool {
	if len(pdoc.Examples) > 0 {
		return true
	}
	for _, f := range pdoc.Funcs {
		if len(f.Examples) > 0 {
			return true
		}
	}
	for _, t := range pdoc.Types {
		if len(t.Examples) > 0 {
			return true
		}
		for _, f := range t.Funcs {
			if len(f.Examples) > 0 {
				return true
			}
		}
		for _, m := range t.Methods {
			if len(m.Examples) > 0 {
				return true
			}
		}

	}
	return false
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
		link := j < len(pdoc.ImportPath) || templateName == "imports.html" || templateName == "importers.html" || templateName == "graph.html"
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

func gaAccountFn() string {
	return secrets.GAAccount
}

func noteTitleFn(s string) string {
	return strings.Title(strings.ToLower(s))
}

func htmlCommentFn(s string) htemp.HTML {
	return htemp.HTML("<!-- " + s + " -->")
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

func joinTemplateDir(baseDir string, files []string) []string {
	result := make([]string, len(files))
	for i := range files {
		result[i] = filepath.Join(baseDir, "templates", files[i])
	}
	return result
}

func parseHTMLTemplates(sets [][]string) error {
	for _, set := range sets {
		templateName := set[0]
		t := htemp.New("")
		t.Funcs(htemp.FuncMap{
			"htmlComment":       htmlCommentFn,
			"breadcrumbs":       breadcrumbsFn,
			"comment":           commentFn,
			"code":              codeFn,
			"equal":             reflect.DeepEqual,
			"hasExamples":       hasExamplesFn,
			"gaAccount":         gaAccountFn,
			"importPath":        importPathFn,
			"isValidImportPath": doc.IsValidPath,
			"map":               mapFn,
			"noteTitle":         noteTitleFn,
			"pageName":          pageNameFn,
			"relativePath":      relativePathFn,
			"relativeTime":      relativeTime,
			"staticFile":        staticFileFn,
			"templateName":      func() string { return templateName },
		})
		if _, err := t.ParseFiles(joinTemplateDir(*baseDir, set)...); err != nil {
			return err
		}
		t = t.Lookup("ROOT")
		if t == nil {
			return fmt.Errorf("ROOT template not found in %v", set)
		}
		templates[set[0]] = t
	}
	return nil
}

func parseTextTemplates(sets [][]string) error {
	for _, set := range sets {
		t := ttemp.New("")
		t.Funcs(ttemp.FuncMap{
			"comment": commentTextFn,
		})
		if _, err := t.ParseFiles(joinTemplateDir(*baseDir, set)...); err != nil {
			return err
		}
		t = t.Lookup("ROOT")
		if t == nil {
			return fmt.Errorf("ROOT template not found in %v", set)
		}
		templates[set[0]] = t
	}
	return nil
}

var presentTemplates = make(map[string]*htemp.Template)

func parsePresentTemplates(sets [][]string) error {
	for _, set := range sets {
		t := present.Template()
		if _, err := t.ParseFiles(joinTemplateDir(*presentBaseDir, set[1:])...); err != nil {
			return err
		}
		t = t.Lookup("root")
		if t == nil {
			return fmt.Errorf("root template not found in %v", set)
		}
		presentTemplates[set[0]] = t
	}
	return nil
}
