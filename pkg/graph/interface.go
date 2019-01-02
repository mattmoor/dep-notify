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
	gb "go/build"
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"
)

// Interface for manipulating the dependency graph.
type Interface interface {
	// This adds a given importpath to the collection of roots that we are tracking.
	Add(importpath string) error

	// Shutdown stops tracking all Add'ed import paths for changes.
	Shutdown() error
}

// Observer is the type for the callback that will happen on changes.
// The callback is supplied with the transitive dependents (aka "affected
// targets") of the file that has changed.
type Observer func(affected StringSet)

// PackageFilter is the type of functions that determine whether to omit a
// particular Go package from the dependency graph.
type PackageFilter func(*gb.Package) bool

// OutsideWorkDir is a functor for creating PackageFilters for excluding packages
// that are outside of the current working directory.
func OutsideWorkDir(workdir string) PackageFilter {
	return func(pkg *gb.Package) bool {
		return !strings.HasPrefix(pkg.Dir, workdir)
	}
}

// OmitVendor implements PackageFilter to exclude packages in the vendor directory.
func OmitVendor(pkg *gb.Package) bool {
	return strings.Contains(pkg.ImportPath, "/vendor/")
}

// FileFilter is the type of functions that determine whether to omit a particular
// file from triggering a package-level event.
type FileFilter func(string) bool

// OmitNonGo implements FileFilter to exclude non-Go files from triggering package
// change notifications.
func OmitNonGo(path string) bool {
	return filepath.Ext(path) != ".go"
}

// OmitTests implements FileFilter to exclude Go test files from triggering package
// change notifications.
func OmitTest(path string) bool {
	return strings.HasSuffix(path, "_test.go")
}

// New creates a new Interface for building up dependency graphs.
// It starts in the provided working directory, and will call the provided
// Observer for any changes.
//
// The returned graph is empty, but new targets may be added via the returned
// Interface.  New also returns any immediate errors, and a channel through which
// errors watching for changes in the dependencies will be returned until the
// graph is Shutdown.
//   // Create our empty graph
//   g, errCh, err := New(...)
//   if err != nil { ... }
//   // Cleanup when we're done.  This closes errCh.
//   defer g.Shutdown()
//   // Start tracking this target.
//   err := g.Add("github.com/mattmoor/warm-image/cmd/controller")
//   if err != nil { ... }
//   select {
//     case err := <- errCh:
//       // Handle errors that occur while watching the above target.
//     case <-stopCh:
//       // When some stop signal happens, we're done.
//   }
func New(workdir string, obs Observer) (Interface, chan error, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, nil, err
	}

	// TODO(mattmoor): Use the builder style and surface functional options that
	// mutate the builder, including ones for specifying custom package and file
	// filters to override these.
	defaultPackageFilters := []PackageFilter{OutsideWorkDir(workdir), OmitVendor}
	defaultFileFilters := []FileFilter{OmitTest, OmitNonGo}

	m := &manager{
		packages:   make(map[string]*node),
		pkgFilters: defaultPackageFilters,
		watcher:    watcher,
		workdir:    workdir,
		errCh:      watcher.Errors,
	}

	// Start listening for events via the filesystem watcher.
	go func() {
		for {
			event, ok := <-watcher.Events
			if !ok {
				// When the channel has been closed, the watcher is shutting down
				// and we should return to cleanup the go routine.
				return
			}

			// Apply our file filters to improve the signal-to-noise.
			skip := false
			for _, f := range defaultFileFilters {
				if f(event.Name) {
					skip = true
				}
			}
			if skip {
				continue
			}

			// Determine what package contains this file
			// and signal the change.  Call our Observer
			// on affected targets when we're done.
			if n := m.enclosingPackage(event.Name); n != nil {
				m.onChange(n, func(n *node) {
					obs(m.affectedTargets(n))
				})
			}
		}
	}()

	return m, watcher.Errors, nil
}
