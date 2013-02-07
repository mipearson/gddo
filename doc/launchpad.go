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
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"io/ioutil"
	"net/http"
	"path"
	"regexp"
	"strings"
)

var launchpadPattern = regexp.MustCompile(`^launchpad\.net/(?P<repo>(?P<project>[a-z0-9A-Z_.\-]+)(?P<series>/[a-z0-9A-Z_.\-]+)?|~[a-z0-9A-Z_.\-]+/(\+junk|[a-z0-9A-Z_.\-]+)/[a-z0-9A-Z_.\-]+)(?P<dir>/[a-z0-9A-Z_.\-/]+)*$`)

func getLaunchpadDoc(client *http.Client, match map[string]string, savedEtag string) (*Package, error) {

	if match["project"] != "" && match["series"] != "" {
		rc, err := httpGet(client, expand("https://code.launchpad.net/{project}{series}/.bzr/branch-format", match), nil)
		switch {
		case err == nil:
			rc.Close()
			// The structure of the import path is launchpad.net/{root}/{dir}.
		case IsNotFound(err):
			// The structure of the import path is is launchpad.net/{project}/{dir}.
			match["repo"] = match["project"]
			match["dir"] = expand("{series}{dir}", match)
		default:
			return nil, err
		}
	}

	p, etag, err := httpGetBytesCompare(client, expand("https://bazaar.launchpad.net/+branch/{repo}/tarball", match), savedEtag)
	if err != nil {
		return nil, err
	}

	gzr, err := gzip.NewReader(bytes.NewReader(p))
	if err != nil {
		return nil, err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	inTree := false
	dirPrefix := expand("+branch/{repo}{dir}/", match)
	var files []*source
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if !strings.HasPrefix(hdr.Name, dirPrefix) {
			continue
		}
		inTree = true
		if d, f := path.Split(hdr.Name); d == dirPrefix && isDocFile(f) {
			b, err := ioutil.ReadAll(tr)
			if err != nil {
				return nil, err
			}
			files = append(files, &source{
				name:      f,
				browseURL: expand("http://bazaar.launchpad.net/+branch/{repo}/view/head:{dir}/{0}", match, f),
				data:      b})
		}
	}

	if !inTree {
		return nil, NotFoundError{"Directory tree does not contain Go files."}
	}

	b := &builder{
		lineFmt: "#L%d",
		pkg: &Package{
			ImportPath:  match["importPath"],
			ProjectRoot: expand("launchpad.net/{repo}", match),
			ProjectName: match["repo"],
			ProjectURL:  expand("https://launchpad.net/{repo}/", match),
			BrowseURL:   expand("http://bazaar.launchpad.net/+branch/{repo}/view/head:{dir}/", match),
			Etag:        etag,
			VCS:         "bzr",
		},
	}
	return b.build(files)
}
