/*
Copyright 2018 Matt Moore

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package graph

import (
	"fmt"
)

// node holds a single node in our dependency graph and its immediately adjacent neighbors.
type node struct {
	// The canonical import path for this node.
	name string

	// The dependency structure
	dependencies []*node
	dependents   []*node
}

// node implements fmt.Stringer
var _ fmt.Stringer = (*node)(nil)

// String implements fmt.Stringer
func (n *node) String() string {
	return fmt.Sprintf(`---
name: %s
depdnt: %v
depdncy: %v`, n.name, names(n.dependents), names(n.dependencies))
}

// names returns the deduplicated and sorted names of the provided nodes.
func names(ns []*node) []string {
	ss := make(StringSet, len(ns))
	for _, n := range ns {
		ss.Add(n.name)
	}
	return ss.InOrder()
}
