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

package main

import (
	"log"
	"os"
	"time"

	"github.com/mattmoor/dep-notify/pkg/graph"
)

func main() {
	// Assume we're invoked from the current directory.
	wd, _ := os.Getwd()

	// Create a new empty graph that simply enumerates the affected
	// targets (import paths) when a file in a tracked directory
	// changes.
	m, errCh, err := graph.New(wd, func(ss graph.StringSet) {
		log.Printf("Affected targets: %v", ss.InOrder())
	})
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	// Clean up the filesystem watcher when we are done.
	defer m.Shutdown()

	// Add things for us to start tracking.
	if err := m.Add("github.com/knative/serving/cmd/controller"); err != nil {
		log.Fatalf("Error adding to graph: %v", err)
	}

	// Use this timer as our fake stop channel.  Typically this would
	// be something that listens for Ctrl+C and the like.
	stopCh := time.After(30 * time.Second)

	// Listen for errors, or our stop signal.
	select {
	case err := <-errCh:
		log.Fatalf("ERROR: %v", err)
	case <-stopCh:
		log.Printf("Done: %v", m)
	}
}
