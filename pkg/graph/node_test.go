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
	"testing"
)

func TestNilDependent(t *testing.T) {
	n := &node{name: "a"}
	if n.addDependent(nil) {
		t.Error("addDependent(nil) = true, wanted false")
	}
}

func TestLinkedList(t *testing.T) {
	nodes := []*node{
		&node{name: "a"},
		&node{name: "b"},
		&node{name: "c"},
		&node{name: "d"},
		&node{name: "e"},
		&node{name: "f"},
	}

	for i := range nodes[1:] {
		previous := nodes[i]
		next := nodes[i+1]

		// The first call should change the dependency structure.
		if chg := previous.addDependent(next); !chg {
			t.Errorf("addDependent(%s) = false, wanted true", next.name)
		}
		// A second call shouldn't result in a change.
		if chg := previous.addDependent(next); chg {
			t.Errorf("addDependent(%s) = true, wanted false", next.name)
		}

		// The first call should change the dependency structure.
		if chg := next.addDependency(previous); !chg {
			t.Errorf("addDependency(%s) = false, wanted true", previous.name)
		}
		// A second call shouldn't result in a change.
		if chg := next.addDependency(previous); chg {
			t.Errorf("addDependency(%s) = true, wanted false", previous.name)
		}
	}

	for i, n := range nodes {
		dependents := n.transitiveDependents()
		if got, want := len(dependents), len(nodes)-i; got != want {
			t.Errorf("len(dependents) = %d, wanted %d", got, want)
		}

		dependencies := n.transitiveDependencies()
		if got, want := len(dependencies), i+1; got != want {
			t.Errorf("len(dependencies) = %d, wanted %d", got, want)
		}
	}
}

func TestDiamondDependency(t *testing.T) {
	// Create a dependency graph that
	// looks like a diamond:
	//   a
	//  /  \
	// b    c
	//  \  /
	//   d
	nodes := []*node{
		&node{name: "a"},
		&node{name: "b"},
		&node{name: "c"},
		&node{name: "d"},
	}

	// Create the diamond structure.
	nodes[0].addDependency(nodes[1])
	nodes[1].addDependent(nodes[0])

	nodes[0].addDependency(nodes[2])
	nodes[2].addDependent(nodes[0])

	nodes[1].addDependency(nodes[3])
	nodes[3].addDependent(nodes[1])

	nodes[2].addDependency(nodes[3])
	nodes[3].addDependent(nodes[2])

	t.Run("transitive foos of 'a'", func(t *testing.T) {
		dependents := nodes[0].transitiveDependents()
		if got, want := len(dependents), 1; got != want {
			t.Errorf("len(dependents) = %d, wanted %d", got, want)
		}

		dependencies := nodes[0].transitiveDependencies()
		if got, want := len(dependencies), 4; got != want {
			t.Errorf("len(dependencies) = %d, wanted %d", got, want)
		}
	})

	t.Run("transitive foos of 'b'", func(t *testing.T) {
		dependents := nodes[1].transitiveDependents()
		if got, want := len(dependents), 2; got != want {
			t.Errorf("len(dependents) = %d, wanted %d", got, want)
		}

		dependencies := nodes[1].transitiveDependencies()
		if got, want := len(dependencies), 2; got != want {
			t.Errorf("len(dependencies) = %d, wanted %d", got, want)
		}
	})

	t.Run("transitive foos of 'c'", func(t *testing.T) {
		dependents := nodes[2].transitiveDependents()
		if got, want := len(dependents), 2; got != want {
			t.Errorf("len(dependents) = %d, wanted %d", got, want)
		}

		dependencies := nodes[2].transitiveDependencies()
		if got, want := len(dependencies), 2; got != want {
			t.Errorf("len(dependencies) = %d, wanted %d", got, want)
		}
	})

	t.Run("transitive foos of 'd'", func(t *testing.T) {
		dependents := nodes[3].transitiveDependents()
		if got, want := len(dependents), 4; got != want {
			t.Errorf("len(dependents) = %d, wanted %d", got, want)
		}

		dependencies := nodes[3].transitiveDependencies()
		if got, want := len(dependencies), 1; got != want {
			t.Errorf("len(dependencies) = %d, wanted %d", got, want)
		}
	})
}

func TestFormatString(t *testing.T) {
	nodes := []*node{
		&node{name: "a"},
		&node{name: "b"},
		&node{name: "c"},
	}

	for i := range nodes[1:] {
		previous := nodes[i]
		next := nodes[i+1]

		// The first call should change the dependency structure.
		if chg := previous.addDependent(next); !chg {
			t.Errorf("addDependent(%s) = false, wanted true", next.name)
		}

		// The first call should change the dependency structure.
		if chg := next.addDependency(previous); !chg {
			t.Errorf("addDependency(%s) = false, wanted true", previous.name)
		}
	}

	t.Run("root format string", func(t *testing.T) {
		want := `---
name: a
depdnt: [b]
depdncy: []`
		got := nodes[0].String()
		if got != want {
			t.Errorf("String() = %q, wanted %q", got, want)
		}
	})

	t.Run("middle format string", func(t *testing.T) {
		want := `---
name: b
depdnt: [c]
depdncy: [a]`
		got := nodes[1].String()
		if got != want {
			t.Errorf("String() = %q, wanted %q", got, want)
		}
	})

	t.Run("leaf format string", func(t *testing.T) {
		want := `---
name: c
depdnt: []
depdncy: [b]`
		got := nodes[2].String()
		if got != want {
			t.Errorf("String() = %q, wanted %q", got, want)
		}
	})
}
