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
	etag := ""
	if pdoc != nil {
		etag = pdoc.Etag
	}

	var d time.Duration
	var err error

	if i := strings.Index(path, "/src/pkg/"); i > 0 && doc.IsGoRepoPath(path[i+len("/src/pkg/"):]) {
		// Go source tree mirror.
		pdoc = nil
		err = doc.NotFoundError{"Go source tree mirror."}
	} else if i := strings.Index(path, "/libgo/go/"); i > 0 && doc.IsGoRepoPath(path[i+len("/libgo/go/"):]) {
		// Go Frontend source tree mirror.
		pdoc = nil
		err = doc.NotFoundError{"Go Frontend source tree mirror."}
	} else if m := nestedProjectPat.FindStringIndex(path); m != nil && exists(path[m[0]+1:]) {
		pdoc = nil
		err = doc.NotFoundError{"Copy of other project."}
	} else if blocked, e := db.IsBlocked(path); blocked && e == nil {
		pdoc = nil
		err = doc.NotFoundError{"Blocked."}
	} else {
		t := time.Now()
		var pdocNew *doc.Package
		pdocNew, err = doc.Get(httpClient, path, etag)
		d = time.Since(t) / time.Millisecond

		// For timeout logic in client.go to work, we cannot leave connections idling. This is ugly.
		httpTransport.CloseIdleConnections()

		if err != doc.ErrNotModified {
			pdoc = pdocNew
		}
	}

	switch {
	case err == nil:
		log.Printf("%s put    %q %q -> %q %dms", source, path, etag, pdoc.Etag, d)
		if err := db.Put(pdoc); err != nil {
			log.Printf("ERROR db.Put(%q): %v", path, err)
		}
	case err == doc.ErrNotModified:
		log.Printf("%s touch  %q %q %dms", source, path, etag, d)
		if err := db.TouchLastCrawl(pdoc); err != nil {
			log.Printf("ERROR db.TouchLastCrawl(%q): %v", path, err)
		}
	case doc.IsNotFound(err):
		pdoc = nil
		log.Printf("%s delete %q %s %dms", source, path, err.Error(), d)
		if err := db.Delete(path); err != nil {
			log.Printf("ERROR db.Delete(%q): %v", path, err)
		}
	default:
		log.Printf("%s error  %q %q %dms %v", source, path, etag, d, err)
		return nil, err
	}

	return pdoc, nil
}

func crawl(interval time.Duration) {
	for {
		time.Sleep(interval)
		pdoc, pkgs, lastCrawl, err := db.Get("-")
		if err != nil {
			log.Printf("db.Get(\"-\") returned error %v", err)
			continue
		}
		if pdoc == nil {
			time.Sleep(*maxAge)
			continue
		}
		sleep := *maxAge - time.Now().Sub(lastCrawl)
		if sleep > 0 {
			time.Sleep(sleep)
		}
		_, err = crawlDoc("crawl", pdoc.ImportPath, pdoc, len(pkgs) > 0)
		if err != nil {
			// Touch package so that crawl advances to next package.
			if err := db.TouchLastCrawl(pdoc); err != nil {
				log.Printf("ERROR db.TouchLastCrawl(%q): %v", pdoc.ImportPath, err)
			}
		}
	}
}
