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
	"testing"
)

var goodImportPaths = []string{
	"github.com/user/repo",
	"camlistore.org",
	"github.com/user/repo/src/pkg/compress/somethingelse",
	"github.com/user/repo/src/compress/gzip",
	"github.com/user/repo/src/pkg",
}

var badImportPaths = []string{
	"foobar",
	"foo.",
	".bar",
	"favicon.ico",
	"github.com/user/repo/testdata/x",
	"github.com/user/repo/_ignore/x",
	"github.com/user/repo/.ignore/x",
	"github.com/user/repo/src/pkg/compress/gzip",
}

func TestValidRemotePath(t *testing.T) {
	for _, importPath := range goodImportPaths {
		if !ValidRemotePath(importPath) {
			t.Errorf("isBadImportPath(%q) -> true, want false", importPath)
		}
	}
	for _, importPath := range badImportPaths {
		if ValidRemotePath(importPath) {
			t.Errorf("isBadImportPath(%q) -> false, want true", importPath)
		}
	}
}

var isBrowseURLTests = []struct {
	s          string
	importPath string
	ok         bool
}{
	{"https://bitbucket.org/user/repo/src/bd0b661a263e/p1/p2?at=default", "bitbucket.org/user/repo/p1/p2", true},
	{"https://bitbucket.org/user/repo/src", "bitbucket.org/user/repo", true},
	{"https://bitbucket.org/user/repo", "bitbucket.org/user/repo", true},
	{"https://github.com/user/repo", "github.com/user/repo", true},
	{"https://github.com/user/repo/tree/master/p1", "github.com/user/repo/p1", true},
}

func TestIsBrowseURL(t *testing.T) {
	for _, tt := range isBrowseURLTests {
		importPath, ok := IsBrowseURL(tt.s)
		if tt.ok {
			if importPath != tt.importPath || ok != true {
				t.Errorf("IsBrowseURL(%q) = %q, %v; want %q %v", tt.s, importPath, ok, tt.importPath, true)
			}
		} else if ok {
			t.Errorf("IsBrowseURL(%q) = %q, %v; want _, false", tt.s, importPath, ok)
		}
	}
}
