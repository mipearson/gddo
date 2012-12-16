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
	"code.google.com/p/goauth2/oauth/jwt"
	"encoding/json"
	"errors"
	"github.com/garyburd/gopkgdoc/database"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	popularMutex    sync.Mutex
	popularPackages []database.Package
)

func updatePopularPackagesOnce() error {
	const n = 25
	c := http.DefaultClient
	o, err := jwt.NewToken(
		secrets.ServiceAccountSecrets.Web.ClientEmail,
		"https://www.googleapis.com/auth/analytics.readonly",
		secrets.serviceAccountPEMBytes).Assert(c)
	if err != nil {
		return err
	}
	q := url.Values{
		"start-date":   {time.Now().Add(-8 * 24 * time.Hour).Format("2006-01-02")},
		"end-date":     {time.Now().Format("2006-01-02")},
		"ids":          {"ga:58440332"},
		"dimensions":   {"ga:pagePath"},
		"metrics":      {"ga:visitors"},
		"sort":         {"-ga:visitors"},
		"filters":      {`ga:previousPagePath!=/;ga:pagePath=~^/[a-z][^.?]*\.[^?]+$`},
		"max-results":  {strconv.Itoa(n + 10)},
		"access_token": {o.AccessToken},
	}
	resp, err := c.Get("https://www.googleapis.com/analytics/v3/data/ga?" + q.Encode())
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var data struct {
		Rows [][]string `json:"rows"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return err
	}
	paths := make([]string, len(data.Rows))
	for i, r := range data.Rows {
		if !strings.HasPrefix(r[0], "/") {
			return errors.New("bad path value " + r[0])
		}
		paths[i] = r[0][1:]
	}
	pkgs, err := db.Packages(paths)
	if err != nil {
		return err
	}

	i := 0
	prev := "-"
	for _, pkg := range pkgs {
		if strings.HasPrefix(pkg.Path, prev) {
			continue
		}
		prev = pkg.Path + "/"
		pkgs[i] = pkg
		i += 1
		if i >= n {
			break
		}
	}
	pkgs = pkgs[:i]

	popularMutex.Lock()
	popularPackages = pkgs
	popularMutex.Unlock()

	return nil
}

func getPopularPackages() []database.Package {
	popularMutex.Lock()
	pkgs := popularPackages
	popularMutex.Unlock()
	return pkgs
}

func updatePopularPackages(interval time.Duration) {
	for {
		if err := updatePopularPackagesOnce(); err != nil {
			log.Printf("Error updating popular packages, %v", err)
		} else {
			log.Print("Popular packages updated.")
		}
		time.Sleep(interval)
	}
}
