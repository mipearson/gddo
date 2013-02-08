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

package main

import (
	"bytes"
	"fmt"
	"os/exec"

	"github.com/garyburd/gopkgdoc/database"
	"github.com/garyburd/gopkgdoc/doc"
)

func renderGraph(pdoc *doc.Package, pkgs []database.Package, edges [][2]int) ([]byte, error) {
	var in, out bytes.Buffer

	fmt.Fprintf(&in, "digraph %s {\n", pdoc.Name)
	for i, pkg := range pkgs {
		fmt.Fprintf(&in, " n%d [label=\"%s\", URL=\"/%s\"];\n", i, pkg.Path, pkg.Path)
	}
	for _, edge := range edges {
		fmt.Fprintf(&in, " n%d -> n%d;\n", edge[0], edge[1])
	}
	in.WriteString("}")

	cmd := exec.Command("dot", "-Tsvg")
	cmd.Stdin = &in
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
