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
	gb "go/build"
	// "log"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// notification is a callback that internal consumers of manager may use to get
// a crack at a node after it has been updated.
type notification func(*node)

// empty implements notification and does nothing.
func empty(n *node) {}

type manager struct {
	m sync.Mutex

	pkgFilters []PackageFilter

	packages map[string]*node
	watcher  interface {
		Add(string) error
		Remove(string) error
		Close() error
	}

	// The working directory relative to which import paths are evaluated.
	workdir string

	errCh chan error
}

// manager implements Interface
var _ Interface = (*manager)(nil)

// manager implements fmt.Stringer
var _ fmt.Stringer = (*manager)(nil)

// Add implements Interface
func (m *manager) Add(importpath string) error {
	m.m.Lock()
	defer m.m.Unlock()

	_, _, err := m.add(importpath, nil)
	return err
}

// add adds the provided importpath (if it doesn't exist) and optionally
// adds the dependent node (if provided) as a dependent of the target.
// This returns the node for the provided importpath, whether the dependency
// structure has changed, and any errors that may have occurred adding the node.
func (m *manager) add(importpath string, dependent *node) (*node, bool, error) {
	// INVARIANT m.m must be held to call this.
	if pkg, ok := m.packages[importpath]; ok {
		return pkg, m.addDependent(pkg, dependent), nil
	}

	// New nodes always start as a simple shell, then we set up the
	// fsnotify and immediate simulate a change to prompt the package
	// to load its data.  A good analogy would be how the "diff" in
	// a code review for new files looks like everything being added;
	// so too does this first simulated change pull in the rest of the
	// dependency graph.
	newNode := &node{
		name: importpath,
	}
	m.packages[importpath] = newNode
	m.addDependent(newNode, dependent)

	// Load the package once to determine it's filesystem location,
	// and set up a watch on that location.
	pkg, err := gb.Import(importpath, m.workdir, gb.ImportComment)
	if err != nil {
		// TODO(mattmoor): Cleanup newNode?
		return nil, false, err
	}
	if err := m.watcher.Add(pkg.Dir); err != nil {
		// TODO(mattmoor): Cleanup newNode?
		return nil, false, err
	}

	// This is done via go routine so that it can take over the lock.
	go m.onChange(newNode, empty)

	return newNode, true, nil
}

// addDependent adds the given dependent to the list of dependents for
// the given dependency.  Dependent may be nil.
// This returns whether the node's neighborhood changes.
// The manager's lock must be held before calling this.
func (m *manager) addDependent(dependency, dependent *node) bool {
	if dependent == nil {
		return false
	}

	for _, depdnt := range dependency.dependents {
		if depdnt == dependent {
			// Already a dependent
			return false
		}
	}

	// log.Printf("Adding %s <- %s", dependent.name, dependency.name)
	dependency.dependents = append(dependency.dependents, dependent)
	return true
}

// addDependency adds the given dependency to the list of dependencies for
// the given dependent.  Neither parameter may be nil.
// This returns whether the node's neighborhood changes.
// The manager's lock must be held before calling this.
func (m *manager) addDependency(dependent, dependency *node) bool {
	for _, depdcy := range dependent.dependencies {
		if depdcy == dependency {
			// Already a dependency
			return false
		}
	}

	// log.Printf("Adding %s -> %s", dependent.name, dependency.name)
	dependent.dependencies = append(dependent.dependencies, dependency)
	return true
}

// affectedTargets returns the set of targets that would be affected by a
// change to the target represented by the given node.  This set is comprised
// of the transitive dependents of the node, including itself.
func (m *manager) affectedTargets(n *node) StringSet {
	m.m.Lock()
	defer m.m.Unlock()

	// We will use "set" as the visited group for our DFS.
	set := make(StringSet)
	queue := []*node{n}

	for len(queue) != 0 {
		// Pop the top element off of the queue.
		top := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		// Check/Mark visited
		if set.Has(top.name) {
			continue
		}
		set.Add(top.name)

		// Append this node's dependents to our search.
		queue = append(queue, top.dependents...)
	}

	return set
}

// enclosingPackage returns the node for the package covering the
// watched path.
func (m *manager) enclosingPackage(path string) *node {
	m.m.Lock()
	defer m.m.Unlock()

	dir := filepath.Dir(path)
	for k, v := range m.packages {
		if strings.HasSuffix(dir, k) {
			return v
		}
	}
	return nil
}

// onChange updates the graph based on the current state of the package
// represented by the given node.  Once the graph has been updated, the
// notification function is called on the node.
func (m *manager) onChange(changed *node, not notification) {
	m.m.Lock()
	defer m.m.Unlock()

	// Load the package information and update dependencies.
	pkg, err := gb.Import(changed.name, m.workdir, gb.ImportComment)
	if err != nil {
		m.errCh <- err
		return
	}

	// haveDepsChanged := false
	for _, ip := range pkg.Imports {
		if ip == "C" {
			// skip cgo
			continue
		}
		subpkg, err := gb.Import(ip, m.workdir, gb.ImportComment)
		if err != nil {
			m.errCh <- err
			return
		}

		skip := false
		for _, f := range m.pkgFilters {
			if f(subpkg) {
				skip = true
				break
			}
		}
		if skip {
			continue
		}

		// // TODO(mattmoor): Create a way to pass in filters instead
		// // of hardcoding these.
		// if !strings.HasPrefix(subpkg.Dir, m.workdir) {
		// 	// Skip import paths outside of our workspace (std library)
		// 	continue
		// }
		// if strings.Contains(subpkg.ImportPath, "/vendor/") {
		// 	// Skip vendor dependencies.
		// 	continue
		// }

		n, chg, err := m.add(subpkg.ImportPath, changed)
		if err != nil {
			m.errCh <- err
			return
		} else if chg {
			// haveDepsChanged = true
		}
		if m.addDependency(changed, n) {
			// haveDepsChanged = true
		}
	}

	// TODO(mattmoor): Remove dependencies that we no longer have.

	// log.Printf("Processing %s, have deps changed: %v", changed.name, haveDepsChanged)
	// Done via go routine so that we can be passed a callback that
	// takes the lock on manager.
	go not(changed)
}

// Shutdown implements Interface.
func (m *manager) Shutdown() error {
	return m.watcher.Close()
}

// String implements fmt.Stringer
func (m *manager) String() string {
	m.m.Lock()
	defer m.m.Unlock()

	// WTB Topo sort.
	order := []string{}
	for k := range m.packages {
		order = append(order, k)
	}
	sort.Strings(order)

	parts := []string{}
	for _, key := range order {
		parts = append(parts, m.packages[key].String())
	}
	return strings.Join(parts, "\n")
}
