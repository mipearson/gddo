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
	"bytes"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strings"
)

// TODO: specify with command line flag
const repoRoot = "/tmp/gddo"

var urlTemplates = []struct {
	re       *regexp.Regexp
	template string
	lineFmt  string
}{
	{
		regexp.MustCompile(`^git\.gitorious\.org/(?P<repo>[^/]+/[^/]+)$`),
		"https://gitorious.org/{repo}/blobs/{tag}/{dir}{0}",
		"#line%d",
	},
	{
		regexp.MustCompile(`^camlistore\.org/r/p/(?P<repo>[^/]+)$`),
		"http://camlistore.org/code/?p={repo}.git;hb={tag};f={dir}{0}",
		"#l%d",
	},
}

// lookupURLTemplate finds an expand() template, match map and line number
// format for well known repositories.
func lookupURLTemplate(repo, dir, tag string) (string, map[string]string, string) {
	if strings.HasPrefix(dir, "/") {
		dir = dir[1:] + "/"
	}
	for _, t := range urlTemplates {
		if m := t.re.FindStringSubmatch(repo); m != nil {
			match := map[string]string{
				"dir": dir,
				"tag": tag,
			}
			for i, name := range t.re.SubexpNames() {
				if name != "" {
					match[name] = m[i]
				}
			}
			return t.template, match, t.lineFmt
		}
	}
	return "", nil, ""
}

type vcsCmd struct {
	schemes  []string
	download func(*http.Client, string, string, string) (string, string, error)
}

var vcsCmds = map[string]*vcsCmd{
	"git": &vcsCmd{
		schemes:  []string{"https", "http"},
		download: downloadGit,
	},
}

var lsremoteRe = regexp.MustCompile(`[0-9a-f]{4}([0-9a-f]{40}) refs/(?:tags|heads)/(.+)\n`)

func downloadGit(client *http.Client, scheme, repo, savedEtag string) (string, string, error) {
	p, err := httpGetBytes(client, scheme+"://"+repo+".git/info/refs?service=git-upload-pack", nil)
	if err != nil {
		return "", "", errNoMatch
	}

	tags := make(map[string]string)
	for _, m := range lsremoteRe.FindAllSubmatch(p, -1) {
		tags[string(m[2])] = string(m[1])
	}
	tag, commit, err := bestTag(tags, "master")
	if err != nil {
		return "", "", err
	}

	if commit == savedEtag {
		return "", "", ErrNotModified
	}

	dir := path.Join(repoRoot, repo+".git")
	p, err = ioutil.ReadFile(path.Join(dir, ".git/HEAD"))
	switch {
	case err != nil:
		if err := os.MkdirAll(dir, 0777); err != nil {
			return "", "", err
		}
		log.Printf("git clone  %s://%s", scheme, repo)
		cmd := exec.Command("git", "clone", scheme+"://"+repo, dir)
		if err := cmd.Run(); err != nil {
			return "", "", err
		}
	case string(bytes.TrimRight(p, "\n")) == commit:
		return tag, commit, nil
	default:
		log.Printf("git fetch %s", repo)
		cmd := exec.Command("git", "fetch")
		cmd.Dir = dir
		if err := cmd.Run(); err != nil {
			return "", "", err
		}
	}

	cmd := exec.Command("git", "checkout", "--detach", "--force", commit)
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		return "", "", err
	}

	return tag, commit, nil
}

func getVCS(client *http.Client, vcs, scheme, repo, dir, etagSaved string) (*Package, error) {
	cmd := vcsCmds[vcs]
	if cmd == nil {
		return nil, NotFoundError{"VCS not supported: " + vcs}
	}

	// Find protocols to use.

	var schemes []string
	if scheme == "" {
		schemes = cmd.schemes
	} else {
		for _, s := range cmd.schemes {
			if s == scheme {
				schemes = []string{scheme}
				break
			}
		}
	}

	// Download and checkout.

	var tag, etag string
	err := errNoMatch
	for _, scheme := range schemes {
		tag, etag, err = cmd.download(client, scheme, repo, etagSaved)
		if err != errNoMatch {
			break
		}
	}

	switch {
	case err == errNoMatch:
		return nil, NotFoundError{"Repository not found."}
	case err != nil:
		return nil, err
	}

	// Find source location.

	urlTemplate, urlMatch, lineFmt := lookupURLTemplate(repo, dir, tag)

	// Slurp source files.

	d := path.Join(repoRoot, repo+"."+vcs, dir)
	f, err := os.Open(d)
	if err != nil {
		if os.IsNotExist(err) {
			err = NotFoundError{err.Error()}
		}
		return nil, err
	}
	fis, err := f.Readdir(-1)
	if err != nil {
		return nil, err
	}

	var files []*source
	for _, fi := range fis {
		if fi.IsDir() || !isDocFile(fi.Name()) {
			continue
		}
		b, err := ioutil.ReadFile(path.Join(d, fi.Name()))
		if err != nil {
			return nil, err
		}
		files = append(files, &source{
			name:      fi.Name(),
			browseURL: expand(urlTemplate, urlMatch, fi.Name()),
			data:      b,
		})
	}

	// Create the documentation.

	b := &builder{
		lineFmt: lineFmt,
		pkg: &Package{
			ImportPath:  repo + "." + vcs + dir,
			ProjectRoot: repo + "." + vcs,
			ProjectName: path.Base(repo),
			ProjectURL:  "",
			BrowseURL:   "",
			Etag:        etag,
			VCS:         vcs,
		},
	}

	return b.build(files)
}
