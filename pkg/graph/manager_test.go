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
	"errors"
	"fmt"
	gb "go/build"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type fake struct {
	a func(string) error
	r func(string) error
	c func() error
}

var _ watcher = (*fake)(nil)

func (f *fake) Add(k string) error {
	return f.a(k)
}

func (f *fake) Remove(k string) error {
	return f.r(k)
}

func (f *fake) Close() error {
	return f.c()
}

func TestShutdownClosesWatcher(t *testing.T) {
	called := false
	m := &manager{watcher: &fake{c: func() error {
		called = true
		return nil
	}}}

	if called {
		t.Error("called = true, wanted false")
	}

	m.Shutdown()

	if !called {
		t.Error("called = false, wanted true")
	}
}

func writeFilesToGOPATH(dir string, n2c map[string]string) error {
	for name, content := range n2c {
		fp := filepath.Join(dir, name)

		dn := filepath.Dir(fp)
		if err := os.MkdirAll(dn, 0777); err != nil {
			return err
		}
		if err := ioutil.WriteFile(fp, []byte(content), 0777); err != nil {
			return err
		}
	}
	return nil
}

// TestBuildGraph constructs a fake GOPATH with files referencing each other and checks
// that the graph is properly updated across several mutations.
func TestBuildGraph(t *testing.T) {
	gopath, err := ioutil.TempDir("", "")
	if err != nil {
		t.Errorf("TempDir() = %v", err)
	}
	defer os.RemoveAll(gopath)

	ip := "github.com/foo/bar"
	workdir := filepath.Join(gopath, "src", ip)

	lib1 := "pkg/blah/stuff.go"
	lib2 := "pkg/bleh/otherstuff.go"
	lib3 := "pkg/blerg/leafyrstuff.go"
	main := "cmd/baz/main.go"

	files := map[string]string{
		lib1: `package blah

import "log"

func Echo() {
	log.Printf("Hello")
}
`,
		lib2: fmt.Sprintf(`package bleh

import "%s/pkg/blerg"

func Echo() {
	blerg.Echo()
}
`, ip),
		lib3: `package blerg

import "log"

func Echo() {
	log.Printf("World")
}
`,
		main: fmt.Sprintf(`package main

import "%s/pkg/blah"

func main() {
	blah.Echo()
}
`, ip),
	}

	if err := writeFilesToGOPATH(workdir, files); err != nil {
		t.Errorf("writeFilesToGOPATH() = %v", err)
	}

	adds := 0
	removes := 0

	ctx := gb.Default
	ctx.GOPATH = gopath

	errCh := make(chan error)
	defer close(errCh)
	go func() {
		err, ok := <-errCh
		if !ok {
			return
		}
		t.Errorf("Error: %v", err)
	}()

	m := &manager{
		ctx:      &ctx,
		packages: make(map[string]*node),
		watcher: &fake{
			a: func(p string) error {
				adds++
				return nil
			},
			r: func(p string) error {
				removes++
				return nil
			},
			c: func() error {
				return nil
			},
		},
		workdir: workdir,
		errCh:   errCh,
	}
	WithOutsideWorkDirFilter(m)
	defer t.Logf("Manager end state: %v", m)
	defer m.Shutdown()

	if err := m.Add(filepath.Join(ip, "cmd/baz")); err != nil {
		t.Errorf("Add() = %v", err)
	}

	// Sleep because we build things up asynchronously.
	time.Sleep(100 * time.Millisecond)

	// We should have seen "watches" added on the binary and library.
	if got, want := adds, 2; got != want {
		t.Errorf("watcher.Add() = %d calls, wanted %d calls", got, want)
	}

	// lib2 isn't tracked yet
	pkg2 := m.enclosingPackage(filepath.Join(workdir, lib2))
	if pkg2 != nil {
		t.Errorf("enclosingPackage(%s) = %v, wanted nil", filepath.Join(workdir, lib2), pkg2)
	}

	cmd := m.enclosingPackage(filepath.Join(workdir, main))
	m.onChange(cmd, func(n *node) {
		ss := m.affectedTargets(n)
		if got, want := len(ss), 1; got != want {
			t.Errorf("onChange(cmd) = %d elements, wanted %d elements: %v", got, want, ss.InOrder())
		}
	})

	pkg := m.enclosingPackage(filepath.Join(workdir, lib1))
	m.maybeGC(pkg) // This should do nothing because we have dependents
	m.onChange(pkg, func(n *node) {
		ss := m.affectedTargets(n)
		if got, want := len(ss), 2; got != want {
			t.Errorf("onChange(cmd) = %d elements, wanted %d elements: %v", got, want, ss.InOrder())
		}
	})

	// Sleep because we build things up asynchronously.
	time.Sleep(100 * time.Millisecond)

	newfiles := map[string]string{
		lib1: fmt.Sprintf(`package blah

import "log"
// This is a new import
import "%s/pkg/bleh"

func Echo() {
	log.Printf("Hello")
	// This is a new call through the new import
	bleh.Echo()
}
`, ip),
	}

	if err := writeFilesToGOPATH(workdir, newfiles); err != nil {
		t.Errorf("writeFilesToGOPATH() = %v", err)
	}

	// Simulate an fsnotify notification of the change to lib1.
	m.onChange(pkg, func(n *node) {
		ss := m.affectedTargets(n)
		if got, want := len(ss), 2; got != want {
			t.Errorf("onChange(cmd) = %d elements, wanted %d elements: %v", got, want, ss.InOrder())
		}
	})

	// Sleep because we build things up asynchronously.
	time.Sleep(100 * time.Millisecond)

	// We should now have seen "watches" added on the binary and library.
	if got, want := adds, 4; got != want {
		t.Errorf("watcher.Add() = %d calls, wanted %d calls", got, want)
	}
	// We should not have seen any Remove calls.
	if got, want := removes, 0; got != want {
		t.Errorf("watcher.Remove() = %d calls, wanted %d calls", got, want)
	}

	// Test that changing the file content back calls Remove
	reallynewfiles := map[string]string{
		lib1: `package blah

import "log"

func Echo() {
	log.Printf("Hello")
}
`,
	}

	if err := writeFilesToGOPATH(workdir, reallynewfiles); err != nil {
		t.Errorf("writeFilesToGOPATH() = %v", err)
	}

	// Simulate an fsnotify notification of the change to lib1.
	m.onChange(pkg, func(n *node) {
		ss := m.affectedTargets(n)
		if got, want := len(ss), 2; got != want {
			t.Errorf("onChange(cmd) = %d elements, wanted %d elements: %v", got, want, ss.InOrder())
		}
	})

	// The Remove happens in a parallel goroutine,
	// so wait slightly longer for good measure.
	time.Sleep(100 * time.Millisecond)

	// We should now have seen 2 removes, the direct reference and the indirect reference.
	if got, want := removes, 2; got != want {
		t.Errorf("watcher.Remove() = %d calls, wanted %d calls", got, want)
	}
}

// Test the various error paths building up the graph.
func TestGoImportErrors(t *testing.T) {
	gopath, err := ioutil.TempDir("", "")
	if err != nil {
		t.Errorf("TempDir() = %v", err)
	}
	defer os.RemoveAll(gopath)

	ip := "github.com/foo/bar"
	workdir := filepath.Join(gopath, "src", ip)

	lib1 := "pkg/blah/stuff.go"
	lib2 := "pkg/bleh/otherstuff.go"
	main := "cmd/baz/main.go"

	files := map[string]string{
		lib1: `package blah

import "log"

func Echo() {
	log.Printf("Hello")
}
`,
		lib2: `package bleh

import "log"

func Echo() {
	log.Printf("Hello")
}
`,
		main: `package main

// This format string is intentionally left to cause a failure.
import "%s/pkg/blah"

func main() {
	blah.Echo()
}
`,
	}

	if err := writeFilesToGOPATH(workdir, files); err != nil {
		t.Errorf("writeFilesToGOPATH() = %v", err)
	}

	ctx := gb.Default
	ctx.GOPATH = gopath

	errCh := make(chan error)
	defer close(errCh)

	// Set these to simulate failures in Add or Remove
	var errAdd error
	var errRemove error
	m := &manager{
		ctx:      &ctx,
		packages: make(map[string]*node),
		watcher: &fake{
			a: func(p string) error {
				return errAdd
			},
			r: func(p string) error {
				return errRemove
			},
			c: func() error {
				return nil
			},
		},
		workdir: workdir,
		errCh:   errCh,
	}
	WithOutsideWorkDirFilter(m)
	defer t.Logf("Manager end state: %v", m)
	defer m.Shutdown()

	if err := m.Add(filepath.Join(ip, "cmd/baz")); err == nil {
		t.Error("Add() = nil, wanted error")
	}

	newfiles := map[string]string{
		main: fmt.Sprintf(`package main

// Fix the format string.
import "%s/pkg/blah"

func main() {
	blah.Echo()
}
`, ip),
	}

	if err := writeFilesToGOPATH(workdir, newfiles); err != nil {
		t.Errorf("writeFilesToGOPATH() = %v", err)
	}

	errAdd = errors.New("trigger a failure during Add()")
	if err := m.Add(filepath.Join(ip, "cmd/baz")); err != errAdd {
		t.Errorf("Add() = %v, wanted %v", err, errAdd)
	}
	errAdd = nil

	if err := m.Add(filepath.Join(ip, "cmd/baz")); err != nil {
		t.Errorf("Add() = %v, wanted %v", err, errAdd)
	}

	time.Sleep(100 * time.Millisecond)

	if err := writeFilesToGOPATH(workdir, files); err != nil {
		t.Errorf("writeFilesToGOPATH() = %v", err)
	}

	cmd := m.enclosingPackage(filepath.Join(workdir, main))
	go m.onChange(cmd, func(n *node) {
		t.Error("This should not be called, we should see an error.")
	})

	select {
	case err := <-errCh:
		if err == nil {
			t.Error("Wanted error, but got nil")
		}
	case <-time.After(2 * time.Second):
		t.Error("Timed out waiting on error.")
	}

	reallynewfiles := map[string]string{
		lib1: `package blah

import "%s/log"

func Echo() {
	log.Printf("Hello")
}
`,
		main: fmt.Sprintf(`package main

// Fix the format string.
import "%s/pkg/blah"

func main() {
	blah.Echo()
}
`, ip),
	}

	if err := writeFilesToGOPATH(workdir, reallynewfiles); err != nil {
		t.Errorf("writeFilesToGOPATH() = %v", err)
	}
	go m.onChange(cmd, func(n *node) {
		t.Error("This should not be called, we should see an error.")
	})

	select {
	case err := <-errCh:
		if err == nil {
			t.Error("Wanted error, but got nil")
		}
	case <-time.After(2 * time.Second):
		t.Error("Timed out waiting on error.")
	}

	// Switch to using lib2, and trigger an error Add'ing the indirect dependency.
	finalnewfiles := map[string]string{
		main: fmt.Sprintf(`package main

// Fix the format string.
import "%s/pkg/bleh"

func main() {
	bleh.Echo()
}
`, ip),
	}

	if err := writeFilesToGOPATH(workdir, finalnewfiles); err != nil {
		t.Errorf("writeFilesToGOPATH() = %v", err)
	}

	errAdd = errors.New("trigger a failure during Add()")
	go m.onChange(cmd, func(n *node) {
		t.Error("This should not be called, we should see an error.")
	})

	select {
	case err := <-errCh:
		if err != errAdd {
			t.Errorf("onChange() = %v, wanted %v", err, errAdd)
		}
	case <-time.After(2 * time.Second):
		t.Error("Timed out waiting on error.")
	}
	errAdd = nil
}

func TestBuildGraphFSNotify(t *testing.T) {
	gopath, err := ioutil.TempDir("", "")
	if err != nil {
		t.Errorf("TempDir() = %v", err)
	}
	defer os.RemoveAll(gopath)

	ip := "github.com/foo/bar"
	workdir := filepath.Join(gopath, "src", ip)

	lib1 := "pkg/blah/stuff.go"
	lib1Autosave := "pkg/blah/#stuff.go#"
	lib1Test := "pkg/blah/stuff_test.go"
	lib2 := "pkg/bleh/otherstuff.go"
	lib3 := "pkg/blerg/leafyrstuff.go"
	main := "cmd/baz/main.go"

	files := map[string]string{
		lib1: `package blah

import "log"

func Echo() {
	log.Printf("Hello")
}
`,
		lib2: fmt.Sprintf(`package bleh

import "%s/pkg/blerg"

func Echo() {
	blerg.Echo()
}
`, ip),
		lib3: `package blerg

import "log"

func Echo() {
	log.Printf("World")
}
`,
		main: fmt.Sprintf(`package main

import "%s/pkg/blah"

func main() {
	blah.Echo()
}
`, ip),
	}

	if err := writeFilesToGOPATH(workdir, files); err != nil {
		t.Errorf("writeFilesToGOPATH() = %v", err)
	}

	ctx := gb.Default
	ctx.GOPATH = gopath

	called := 0

	var wantAffectedTargets StringSet
	m, errCh, err := NewWithOptions(func(ss StringSet) {
		called++
		if wantAffectedTargets == nil {
			t.Errorf("Not expecting a call, but got: %s", ss.InOrder())
			return
		}

		if got, want := len(ss), len(wantAffectedTargets); got != want {
			t.Errorf("len(ss) = %d, wanted %d", got, want)
			return
		}

		for _, key := range ss.InOrder() {
			if !wantAffectedTargets.Has(key) {
				t.Errorf("Unexpected affected target: %s", key)
			}
		}

		for _, key := range wantAffectedTargets.InOrder() {
			if !ss.Has(key) {
				t.Errorf("Missing target: %s", key)
			}
		}
	}, append(DefaultOptions, WithWorkDir(workdir), WithContext(&ctx))...)
	if err != nil {
		t.Errorf("NewWithOptions() = %v", err)
	}
	defer t.Logf("Manager end state: %v", m)
	defer m.Shutdown()
	go func() {
		err, ok := <-errCh
		if !ok {
			return
		}
		t.Errorf("Error: %v", err)
	}()

	if err := m.Add(filepath.Join(ip, "cmd/baz")); err != nil {
		t.Errorf("Add() = %v", err)
	}

	// Sleep because we build things up asynchronously.
	time.Sleep(100 * time.Millisecond)

	newfiles := map[string]string{
		lib1: fmt.Sprintf(`package blah

import "log"
// This is a new import
import "%s/pkg/bleh"

func Echo() {
	log.Printf("Hello")
	// This is a new call through the new import
	bleh.Echo()
}
`, ip),
	}

	// We expect these affected targets.
	wantAffectedTargets = make(StringSet)
	wantAffectedTargets.Add(filepath.Join(ip, filepath.Dir(lib1)))
	wantAffectedTargets.Add(filepath.Join(ip, filepath.Dir(main)))

	if err := writeFilesToGOPATH(workdir, newfiles); err != nil {
		t.Errorf("writeFilesToGOPATH() = %v", err)
	}

	// Sleep because we build things up asynchronously.
	time.Sleep(100 * time.Millisecond)

	if called == 0 {
		t.Error("Wanted the observer to get called, but saw no calls")
	}

	morenewfiles := map[string]string{
		lib1Autosave: "shouldn't matter",
		lib1Test:     "shouldn't matter",
	}

	// We don't expect any calls
	wantAffectedTargets = nil

	if err := writeFilesToGOPATH(workdir, morenewfiles); err != nil {
		t.Errorf("writeFilesToGOPATH() = %v", err)
	}

	// Sleep because we build things up asynchronously.
	time.Sleep(100 * time.Millisecond)
}

func TestNewWithFailingOption(t *testing.T) {
	m, _, err := NewWithOptions(func(ss StringSet) {},
		append(DefaultOptions, func(m *manager) error {
			return errors.New("blah")
		})...)
	if err == nil {
		t.Errorf("NewWithOptions() = %v, wanted error", m)
	}
}

// TODO(mattmoor): Import error on Remove
// TODO(mattmoor): Error calling Remove
