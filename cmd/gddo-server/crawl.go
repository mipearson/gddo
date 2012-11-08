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

package main

import (
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/garyburd/gopkgdoc/doc"
)

var nestedProjectPat = regexp.MustCompile(`/(?:github\.com|launchpad\.net|code\.google\.com/p|bitbucket\.org|labix\.org)/`)

func exists(path string) bool {
	b, err := db.Exists(path)
	if err != nil {
		b = false
	}
	return b
}

// crawlDoc fetches the package documentation from the VCS and updates the database.
func crawlDoc(source string, path string, pdoc *doc.Package, hasSubdirs bool) (*doc.Package, error) {
	const (
		put = iota
		del
		touch
	)
	action := put

	etag := ""
	if pdoc != nil {
		etag = pdoc.Etag
	}

	var d time.Duration

	if i := strings.Index(path, "/src/pkg/"); i > 0 && doc.IsGoRepoPath(path[i+len("/src/pkg/"):]) {
		// Go source tree mirror.
		pdoc = nil
		action = del
	} else if i := strings.Index(path, "/libgo/go/"); i > 0 && doc.IsGoRepoPath(path[i+len("/libgo/go/"):]) {
		// Go Frontend source tree mirror.
		pdoc = nil
		action = del
	} else if m := nestedProjectPat.FindStringIndex(path); m != nil && exists(path[m[0]+1:]) {
		// Suffix of path matches another package.
		pdoc = nil
		action = del
	} else {
		t := time.Now()
		pdocNew, err := doc.Get(httpClient, path, etag)
		d = time.Since(t) / time.Millisecond

		// For timeout logic in client.go to work, we cannot leave connections idling. This is ugly.
		httpTransport.CloseIdleConnections()

		switch err {
		case doc.ErrPackageNotModified:
			action = touch
		case doc.ErrPackageNotFound:
			pdoc = nil
			action = del
		case nil:
			pdoc = pdocNew
			action = put
		default:
			log.Printf("%s error  %q %q %dms %v", source, path, etag, d, err)
			return nil, err
		}

		if pdoc != nil && !hasSubdirs {
			if pdoc.Name == "" {
				// Handle directories with no child directories as not found.
				pdoc = nil
				action = del
			} else if pdoc.IsCmd && pdoc.Synopsis == "" {
				///Don't store commands with no documentation and no children.
				action = del
			}
		}
	}

	switch action {
	case put:
		log.Printf("%s put    %q %q %dms", source, path, etag, d)
		if err := db.Put(pdoc); err != nil {
			log.Printf("ERROR db.Put(%q): %v", path, err)
		}
	case touch:
		log.Printf("%s touch  %q %q %dms", source, path, etag, d)
		if err := db.TouchLastCrawl(path); err != nil {
			log.Printf("ERROR db.TouchLastCrawl(%q): %v", path, err)
		}
	case del:
		log.Printf("%s delete %q %q %dms", source, path, etag, d)
		if err := db.Delete(path); err != nil {
			log.Printf("ERROR db.Delete(%q): %v", path, err)
		}
	default:
		panic("should not get here")
	}

	return pdoc, nil
}

func crawl() {
	for {
		time.Sleep(*crawlInterval)
		pdoc, pkgs, lastCrawl, err := db.Get("-")
		if err != nil {
			log.Printf("db.Get(-) returned error %v", err)
			continue
		}
		sleep := *maxAge - time.Now().Sub(lastCrawl)
		if sleep > 0 {
			time.Sleep(sleep)
		}
		pdocNew, err := crawlDoc("crawl", pdoc.ImportPath, pdoc, len(pkgs) > 0)
		if err != nil {
			// Touch to avoid repating errors.
			if err := db.TouchLastCrawl(pdoc.ImportPath); err != nil {
				log.Printf("ERROR db.TouchLastCrawl(%q): %v", pdoc.ImportPath, err)
			}
		} else if pdocNew == nil {
			// nothing for now
			log.Println("Crawl not found", pdoc.ImportPath)
		} else if strings.HasPrefix(pdoc.ImportPath, "github.com/") && pdoc.Etag == pdocNew.Etag {
			// To do:
			//  handle other VCSs
			//  fast touch on crawl from web request.
			pkgs, err := db.Project(pdoc.ProjectRoot)
			if err != nil {
				continue
			}
			for _, pkg := range pkgs {
				if pkg.Path == pdoc.ImportPath {
					continue
				}
				pdocNew, _, _, err := db.Get(pkg.Path)
				if err != nil || pdocNew == nil {
					continue
				}
				if pdocNew.Etag == pdoc.Etag {
					log.Printf("fast  touch  %q %q", pdocNew.ImportPath, pdocNew.Etag)
					if err := db.TouchLastCrawl(pdocNew.ImportPath); err != nil {
						log.Printf("ERROR db.TouchLastCrawl(%q): %v", pdoc.ImportPath, err)
					}
				}
			}
		}
	}
}
