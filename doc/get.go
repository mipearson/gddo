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

// Package doc fetches Go package documentation from version control services.
package doc

import (
	"encoding/xml"
	"errors"
	"net/http"
	"path"
	"regexp"
	"strings"
)

type NotFoundError struct {
	Message string
}

func (e NotFoundError) Error() string {
	return e.Message
}

func IsNotFound(err error) bool {
	_, ok := err.(NotFoundError)
	return ok
}

type RemoteError struct {
	Host string
	err  error
}

func (e *RemoteError) Error() string {
	return e.err.Error()
}

var (
	ErrNotModified = errors.New("package not modified")
	errNoMatch     = errors.New("no match")
)

// service represents a source code control service.
type service struct {
	pattern *regexp.Regexp
	getDoc  func(*http.Client, map[string]string, string) (*Package, error)
	prefix  string
}

// services is the list of source code control services handled by gopkgdoc.
var services = []*service{
	{githubPattern, getGithubDoc, "github.com/"},
	{googlePattern, getGoogleDoc, "code.google.com/"},
	{bitbucketPattern, getBitbucketDoc, "bitbucket.org/"},
	{launchpadPattern, getLaunchpadDoc, "launchpad.net/"},
	{generalPattern, getGeneralDoc, ""},
}

func attrValue(attrs []xml.Attr, name string) string {
	for _, a := range attrs {
		if strings.EqualFold(a.Name.Local, name) {
			return a.Value
		}
	}
	return ""
}

type meta struct {
	projectRoot, projectName, projectURL, repo, vcs string
}

func fetchMeta(client *http.Client, importPath string) (*meta, error) {
	uri := importPath
	if !strings.Contains(uri, "/") {
		// Add slash for root of domain.
		uri = uri + "/"
	}
	uri = uri + "?go-get=1"

	scheme := "https"
	resp, err := client.Get(scheme + "://" + uri)
	if err != nil || resp.StatusCode != 200 {
		if err == nil {
			resp.Body.Close()
		}
		scheme = "http"
		resp, err = client.Get(scheme + "://" + uri)
		if err != nil {
			return nil, &RemoteError{strings.SplitN(importPath, "/", 2)[0], err}
		}
	}
	defer resp.Body.Close()

	var m *meta

	d := xml.NewDecoder(resp.Body)
	d.Strict = false
metaScan:
	for {
		t, tokenErr := d.Token()
		if tokenErr != nil {
			break metaScan
		}
		switch t := t.(type) {
		case xml.EndElement:
			if strings.EqualFold(t.Name.Local, "head") {
				break metaScan
			}
		case xml.StartElement:
			if strings.EqualFold(t.Name.Local, "body") {
				break metaScan
			}
			if !strings.EqualFold(t.Name.Local, "meta") ||
				attrValue(t.Attr, "name") != "go-import" {
				continue metaScan
			}
			f := strings.Fields(attrValue(t.Attr, "content"))
			if len(f) != 3 ||
				!strings.HasPrefix(importPath, f[0]) ||
				!(len(importPath) == len(f[0]) || importPath[len(f[0])] == '/') {
				continue metaScan
			}
			if m != nil {
				return nil, NotFoundError{"More than one <meta> found at " + resp.Request.URL.String()}
			}
			m = &meta{
				projectRoot: f[0],
				vcs:         f[1],
				repo:        f[2],
				projectName: path.Base(f[0]),
				projectURL:  scheme + "://" + f[0],
			}
		}
	}
	if m == nil {
		return nil, NotFoundError{"<meta> not found."}
	}
	return m, nil
}

// getDynamic gets a document from a service that is not statically known.
func getDynamic(client *http.Client, importPath string, etag string) (*Package, error) {
	m, err := fetchMeta(client, importPath)
	if err != nil {
		return nil, err
	}

	if m.projectRoot != importPath {
		mRoot, err := fetchMeta(client, m.projectRoot)
		if err != nil {
			return nil, err
		}
		if mRoot.projectRoot != m.projectRoot {
			return nil, NotFoundError{"Project root mismatch."}
		}
	}

	i := strings.Index(m.repo, "://")
	if i < 0 {
		return nil, NotFoundError{"Bad repo URL in <meta>."}
	}
	scheme := m.repo[:i]
	repo := m.repo[i+len("://"):]
	dir := importPath[len(m.projectRoot):]

	pdoc, err := getStatic(client, repo+dir, etag)
	if err == errNoMatch {
		pdoc, err = getVCS(client, m.vcs, scheme, repo, dir, etag)
	}

	if pdoc != nil {
		pdoc.ImportPath = importPath
		pdoc.ProjectRoot = m.projectRoot
		pdoc.ProjectName = m.projectName
		pdoc.ProjectURL = m.projectURL
	}

	return pdoc, err
}

var generalPattern = regexp.MustCompile(`^(?P<repo>(?:[a-z0-9.\-]+\.)+[a-z0-9.\-]+(?::[0-9]+)?/[A-Za-z0-9_.\-/]*?)\.(?P<vcs>bzr|git|hg|svn)(?P<dir>/[A-Za-z0-9_.\-/]*)?$`)

func getGeneralDoc(client *http.Client, match map[string]string, etag string) (*Package, error) {
	return getVCS(client, match["vcs"], "", match["repo"], match["dir"], etag)
}

// getStatic gets a document from a statically known service. getStatic
// returns errNoMatch if the import path is not recognized.
func getStatic(client *http.Client, importPath string, etag string) (*Package, error) {
	for _, s := range services {
		if !strings.HasPrefix(importPath, s.prefix) {
			continue
		}
		m := s.pattern.FindStringSubmatch(importPath)
		if m == nil {
			if s.prefix != "" {
				return nil, NotFoundError{"Import path prefix matches known service, but regexp does not."}
			}
			continue
		}
		match := map[string]string{"importPath": importPath}
		for i, n := range s.pattern.SubexpNames() {
			if n != "" {
				match[n] = m[i]
			}
		}
		return s.getDoc(client, match, etag)
	}
	return nil, errNoMatch
}

func Get(client *http.Client, importPath string, etag string) (pdoc *Package, err error) {

	const versionPrefix = PackageVersion + "-"

	if strings.HasPrefix(etag, versionPrefix) {
		etag = etag[len(versionPrefix):]
	} else {
		etag = ""
	}

	switch {
	case IsGoRepoPath(importPath):
		pdoc, err = getStandardDoc(client, importPath, etag)
	case IsValidRemotePath(importPath):
		pdoc, err = getStatic(client, importPath, etag)
		if err == errNoMatch {
			pdoc, err = getDynamic(client, importPath, etag)
		}
	default:
		err = errNoMatch
	}

	if err == errNoMatch {
		err = NotFoundError{"Import path not valid."}
	}

	if pdoc != nil {
		pdoc.Etag = versionPrefix + pdoc.Etag
	}

	return pdoc, err
}
