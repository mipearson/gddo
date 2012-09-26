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

// Package index implements GoPkgDoc's backend index.
package index

import (
	"crypto/md5"
	"crypto/rand"
	"github.com/garyburd/gopkgdoc/doc"
	"io"
	"path"
	"sort"
	"strings"
	"sync"
)

// Result is a search result.
type Result struct {
	ImportPath string
	Synopsis   string
	IsCmd      bool
	Score      float32
}

type results []Result

func (r results) Len() int      { return len(r) }
func (r results) Swap(i, j int) { r[i], r[j] = r[j], r[i] }

type byScore struct{ results }

func (r byScore) Less(i, j int) bool { return r.results[i].Score > r.results[j].Score }

type byImportPath struct{ results }

func (r byImportPath) Less(i, j int) bool { return r.results[i].ImportPath < r.results[j].ImportPath }

type identifier uint16

type posting struct {
	// id is the index of a package in Index.pkgs
	id identifier
}

// postingList is a set of package identifiers. 
type postingList []posting

// intersect intersects pl with other and appends the result to dst.
func (pl postingList) intersect(dst, other postingList) postingList {
	if len(pl) == 0 || len(other) == 0 {
		return dst
	}
	var i, j int
	for i < len(pl) && j < len(other) {
		switch {
		case pl[i].id == other[j].id:
			dst = append(dst, pl[i])
			i += 1
			j += 1
		case pl[i].id > other[j].id:
			j += 1
		default:
			i += 1
		}
	}
	return dst
}

// add adds p to pl.
func (pl postingList) add(p posting) postingList {
	if len(pl) == 0 || p.id > pl[len(pl)-1].id {
		return append(pl, p)
	}
	i := sort.Search(len(pl), func(i int) bool { return pl[i].id >= p.id })
	if i < len(pl) && pl[i].id == p.id {
		pl[i] = p
		return pl
	}
	pl = append(pl, p)
	copy(pl[i+1:], pl[i:])
	pl[i] = p
	return pl
}

// remove removes p from pl.
func (pl postingList) remove(p posting) postingList {
	i := sort.Search(len(pl), func(i int) bool { return pl[i].id >= p.id })
	if i >= len(pl) || pl[i].id != p.id {
		return pl
	}
	copy(pl[i:], pl[i+1:])
	return pl[:len(pl)-1]
}

const termSize = md5.Size

// term is a zero padded string or the md5 hash of (salt + string) if the
// string length is greater than the size of an md5 hash. The salt protects
// against intentional collisions by evil package authors.
type term [termSize]byte

var termSalt = readSalt()

func readSalt() []byte {
	p := make([]byte, termSize)
	_, err := io.ReadFull(rand.Reader, p)
	if err != nil {
		panic(err)
	}
	return p
}

func makeTerm(s string) (t term) {
	s = strings.ToLower(s)
	if len(s) <= termSize {
		copy(t[:], s)
	} else {
		h := md5.New()
		h.Write(termSalt)
		io.WriteString(h, s)
		h.Sum(t[:0])
	}
	return
}

func addPackageTerms(terms map[term]int, mask int, dpkg *doc.Package) {
	if dpkg.Name == "" {
		// No terms for empty directories.
		return
	}

	term := makeTerm("project:" + dpkg.ProjectRoot)
	terms[term] = terms[term] | mask

	switch dpkg.IsCmd {
	case true:
		i := strings.Index(dpkg.Doc, ".")
		if dpkg.Synopsis == "" || i <= len(dpkg.Doc)-1 {
			// Synopsis and more than one sentence of documetnation required
			// for commands.
			return
		}
	case false:
		if len(dpkg.Consts) == 0 && len(dpkg.Vars) == 0 && len(dpkg.Funcs) == 0 && len(dpkg.Types) == 0 {
			// At least one export required for packages.
			return
		}
	}

	for _, importPath := range dpkg.Imports {
		term = makeTerm("import:" + importPath)
		terms[term] = terms[term] | mask
	}

	term = makeTerm(dpkg.Name)
	terms[term] = terms[term] | mask

	_, name := path.Split(dpkg.ImportPath)
	term = makeTerm(name)
	terms[term] = terms[term] | mask

	// TODO: add terms from synopsis. Use stop words and stemming.
}

// ipackage is the index package's representation of a package.
type ipackage struct {
	result Result
	dpkg   *doc.Package // TODO: store compressed gob
}

type Index struct {
	mu sync.RWMutex

	// All packages, indexed by id.
	pkgs []*ipackage

	// Map from import path to id.
	ids map[string]identifier

	// Packages containing a given term.
	rindex map[term]postingList
}

// New returns an initialized Index.
func New() *Index {
	return &Index{
		rindex: make(map[term]postingList),
		ids:    make(map[string]identifier),
	}
}

// Put adds or replaces a package in the index.
func (idx *Index) Put(dpkg *doc.Package) error {

	// TODO: improve score calculation.
	var score float32
	switch {
	case dpkg.ProjectRoot == "":
		// standard packages.
		score = 3
	case len(dpkg.Errors) > 0:
		score = 0
	case len(dpkg.Synopsis) == 0:
		score = 1
	default:
		score = 2
	}

	pkg := &ipackage{
		result: Result{
			ImportPath: dpkg.ImportPath,
			IsCmd:      dpkg.IsCmd,
			Score:      score,
			Synopsis:   dpkg.Synopsis,
		},
		dpkg: dpkg,
	}

	const (
		addMask    = 1
		removeMask = 2
	)

	terms := make(map[term]int)
	addPackageTerms(terms, addMask, dpkg)

	idx.mu.Lock()
	defer idx.mu.Unlock()

	id, ok := idx.ids[dpkg.ImportPath]
	if ok {
		if pkg := idx.pkgs[id]; pkg != nil {
			addPackageTerms(terms, removeMask, pkg.dpkg)
		}
		idx.pkgs[id] = pkg
	} else {
		id = identifier(len(idx.pkgs))
		idx.pkgs = append(idx.pkgs, pkg)
		idx.ids[dpkg.ImportPath] = id
	}

	for term, mask := range terms {
		switch mask {
		case addMask:
			idx.rindex[term] = idx.rindex[term].add(posting{id})
		case removeMask:
			idx.rindex[term] = idx.rindex[term].remove(posting{id})
		}
	}
	return nil
}

// Get gets a package from the index.
func (idx *Index) Get(importPath string) (*doc.Package, error) {
	if id, ok := idx.ids[importPath]; ok {
		if pkg := idx.pkgs[id]; pkg != nil {
			return pkg.dpkg, nil
		}
	}
	return nil, doc.ErrPackageNotFound
}

// Remove removes a package form the index.
func (idx *Index) Remove(importPath string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	if id, ok := idx.ids[importPath]; ok {
		if pkg := idx.pkgs[id]; pkg != nil {
			terms := make(map[term]int)
			addPackageTerms(terms, 0, pkg.dpkg)
			for term, _ := range terms {
				idx.rindex[term] = idx.rindex[term].remove(posting{id})
			}
			idx.pkgs[id] = nil
		}
	}
}

func (idx *Index) results(pl postingList) []Result {
	results := make([]Result, 0, len(pl))
	for _, p := range pl {
		if pkg := idx.pkgs[p.id]; pkg != nil {
			results = append(results, pkg.result)
		}
	}
	return results
}

const (
	SortByNone = iota
	SortByPath
	SortByScore
)

// Query queries the index. 
func (idx *Index) Query(q string, sortBy int) ([]Result, error) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	var results []Result

	if q == "all:" {
		results = make([]Result, 0, len(idx.pkgs))
		for _, pkg := range idx.pkgs {
			if pkg != nil && pkg.dpkg.Name != "" {
				results = append(results, pkg.result)
			}
		}
	} else {
		pl := make(postingList, 0)
		fields := strings.Fields(q) // TODO: improve query parser
		switch len(fields) {
		case 0:
			// nothing
		case 1:
			pl = idx.rindex[makeTerm(fields[0])]
		default:
			pl = idx.rindex[makeTerm(fields[0])].intersect(pl, idx.rindex[makeTerm(fields[1])])
			for i := 2; i < len(fields); i++ {
				pl = pl.intersect(pl[:0], idx.rindex[makeTerm(fields[1])])
			}
		}
		results = idx.results(pl)
	}

	switch sortBy {
	case SortByPath:
		sort.Sort(byImportPath{results})
	case SortByScore:
		sort.Sort(byScore{results})
	}
	return results, nil
}

// Subdirs returns child packages for the given import path.
func (idx *Index) Subdirs(importPath string) ([]Result, error) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	prefix := importPath + "/"
	var results []Result

	// Loop looking for project root.
	for importPath != "" {
		if pl, ok := idx.rindex[makeTerm("project:"+importPath)]; ok {
			results = make([]Result, 0, len(pl))
			// Filter project packages to children.
			for _, p := range pl {
				if pkg := idx.pkgs[p.id]; pkg != nil && strings.HasPrefix(pkg.result.ImportPath, prefix) && pkg.dpkg.Name != "" {
					results = append(results, pkg.result)
				}
			}
			break
		}
		i := strings.LastIndex(importPath, "/")
		if i < 0 {
			i = 0
		}
		importPath = importPath[:i]
	}
	return results, nil
}
