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
	"io"
	"time"

	"code.google.com/p/go.talks/pkg/present"
)

type Presentation struct {
	Doc     *present.Doc
	Updated time.Time
}

type presBuilder struct {
	pres       *Presentation
	filename   string
	content    io.Reader
	openFile   func(fname string) (io.ReadCloser, error)
	resolveURL func(fname string) string
}

func (b *presBuilder) resolveElem(e present.Elem) present.Elem {
	switch e := e.(type) {
	case present.Section:
		for i := range e.Elem {
			e.Elem[i] = b.resolveElem(e.Elem[i])
		}
		return e
	case present.Image:
		e.URL = b.resolveURL(e.URL)
		return e
	case present.Iframe:
		e.URL = b.resolveURL(e.URL)
		return e
	case present.HTML:
		// TODO: sanitize HTML
		e.HTML = "HTML not supported on godoc.org"
	}
	return e
}

func (b *presBuilder) build() (*Presentation, error) {
	ctxt := &present.Context{
		OpenFile: b.openFile,
	}

	var err error
	b.pres.Doc, err = present.Parse(ctxt, b.content, b.filename, 0)
	if err != nil {
		return nil, err
	}

	for i := range b.pres.Doc.Sections {
		b.pres.Doc.Sections[i] = b.resolveElem(b.pres.Doc.Sections[i]).(present.Section)
	}

	b.pres.Updated = time.Now().UTC()

	return b.pres, nil
}
