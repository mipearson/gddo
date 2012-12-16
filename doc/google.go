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

package doc

import (
	"errors"
	"net/http"
	"regexp"
	"strings"
)

var (
	googleRepoRe     = regexp.MustCompile(`id="checkoutcmd">(hg|git|svn)`)
	googleRevisionRe = regexp.MustCompile(`<h2>(?:[^ ]+ - )?Revision *([^:]+):`)
	googleEtagRe     = regexp.MustCompile(`^(hg|git|svn)-`)
	googleFileRe     = regexp.MustCompile(`<li><a href="([^"/]+)"`)
	googlePattern    = regexp.MustCompile(`^code\.google\.com/p/(?P<repo>[a-z0-9\-]+)(:?\.(?P<subrepo>[a-z0-9\-]+))?(?P<dir>/[a-z0-9A-Z_.\-/]+)?$`)
)

func getGoogleDoc(client *http.Client, match map[string]string, savedEtag string) (*Package, error) {

	if s := match["subrepo"]; s != "" {
		match["dot"] = "."
		match["query"] = "?repo=" + s
	} else {
		match["dot"] = ""
		match["query"] = ""
	}

	if m := googleEtagRe.FindStringSubmatch(savedEtag); m != nil {
		match["vcs"] = m[1]
	} else {
		// Scrape the HTML project page to find the VCS.
		p, err := httpGetBytes(client, expand("http://code.google.com/p/{repo}/source/checkout", match))
		if err != nil {
			return nil, err
		}
		if m := googleRepoRe.FindSubmatch(p); m != nil {
			match["vcs"] = string(m[1])
		} else {
			return nil, NotFoundError{"Could not VCS on Google Code project page."}
		}
	}

	// Scrape the repo browser to find the project revision and individual Go files.
	p, err := httpGetBytes(client, expand("http://{subrepo}{dot}{repo}.googlecode.com/{vcs}{dir}/", match))
	if err != nil {
		return nil, err
	}

	var etag string
	if m := googleRevisionRe.FindSubmatch(p); m == nil {
		return nil, errors.New("Could not find revision for " + match["importPath"])
	} else {
		etag = expand("{vcs}-{0}", match, string(m[1]))
		if etag == savedEtag {
			return nil, ErrNotModified
		}
	}

	var files []*source
	for _, m := range googleFileRe.FindAllSubmatch(p, -1) {
		fname := string(m[1])
		if isDocFile(fname) {
			files = append(files, &source{
				name:      fname,
				browseURL: expand("http://code.google.com/p/{repo}/source/browse{dir}/{0}{query}", match, fname),
				rawURL:    expand("http://{subrepo}{dot}{repo}.googlecode.com/{vcs}{dir}/{0}", match, fname),
			})
		}
	}

	if err := fetchFiles(client, files, nil); err != nil {
		return nil, err
	}

	b := &builder{
		lineFmt: "#%d",
		pkg: &Package{
			ImportPath:  match["importPath"],
			ProjectRoot: expand("code.google.com/p/{repo}{dot}{subrepo}", match),
			ProjectName: expand("{repo}{dot}{subrepo}", match),
			ProjectURL:  expand("https://code.google.com/p/{repo}/", match),
			BrowseURL:   expand("http://code.google.com/p/{repo}/source/browse{dir}/{query}", match),
			Etag:        etag,
		},
	}

	return b.build(files)
}

func getStandardDoc(client *http.Client, importPath string, savedEtag string) (*Package, error) {

	p, err := httpGetBytes(client, "http://go.googlecode.com/hg-history/release/src/pkg/"+importPath+"/")
	if err != nil {
		return nil, err
	}

	var etag string
	if m := googleRevisionRe.FindSubmatch(p); m == nil {
		return nil, errors.New("Could not find revision for " + importPath)
	} else {
		etag = string(m[1])
		if etag == savedEtag {
			return nil, ErrNotModified
		}
	}

	var files []*source
	for _, m := range googleFileRe.FindAllSubmatch(p, -1) {
		fname := strings.Split(string(m[1]), "?")[0]
		if isDocFile(fname) {
			files = append(files, &source{
				name:      fname,
				browseURL: "http://code.google.com/p/go/source/browse/src/pkg/" + importPath + "/" + fname + "?name=release",
				rawURL:    "http://go.googlecode.com/hg-history/release/src/pkg/" + importPath + "/" + fname,
			})
		}
	}

	if err := fetchFiles(client, files, nil); err != nil {
		return nil, err
	}

	b := &builder{
		lineFmt: "#%d",
		pkg: &Package{
			ImportPath:  importPath,
			ProjectRoot: "",
			ProjectName: "Go",
			ProjectURL:  "https://code.google.com/p/go/",
			BrowseURL:   "http://code.google.com/p/go/source/browse/src/pkg/" + importPath + "?name=release",
			Etag:        etag,
		},
	}

	return b.build(files)
}
