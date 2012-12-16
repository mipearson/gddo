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
	"io/ioutil"
	"path/filepath"
)

// GetDir gets the documentation for the package in dir.
func GetDir(dir string) (*Package, error) {
	fis, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var files []*source
	for _, fi := range fis {
		if fi.IsDir() || !isDocFile(fi.Name()) {
			continue
		}
		b, err := ioutil.ReadFile(filepath.Join(dir, fi.Name()))
		if err != nil {
			return nil, err
		}
		files = append(files, &source{
			name: fi.Name(),
			data: b,
		})
	}
	b := &builder{
		pkg: &Package{
			ImportPath:  "example.com/project/package",
			ProjectRoot: "example.com/project",
			ProjectName: "pacakge",
			ProjectURL:  "http://example.com/project",
			Etag:        "e-t-a-g",
		},
	}
	return b.build(files)
}
