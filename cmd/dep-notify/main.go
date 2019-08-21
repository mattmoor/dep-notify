/*
Copyright 2019 Matt Moore

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
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/mattmoor/dep-notify/pkg/graph"
)

func main() {
	// Create a new empty graph that prints the affected targets (import paths)
	// when a file in a tracked directory changes.
	m, errCh, err := graph.New(func(ss graph.StringSet) {
		for _, target := range ss.InOrder() {
			fmt.Println(target)
		}
	})
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	// Clean up the filesystem watcher when we are done.
	defer m.Shutdown()

	// Read packages to watch from stdin.
	stdin := bufio.NewReader(os.Stdin)
	for {
		pkg, err := stdin.ReadString('\n')
		if err == io.EOF {
			break
		} else if err != nil {
			log.Fatalf("Error when reading from stdin: %v", err)
		}
		pkg = strings.TrimSpace(pkg)
		if err := m.Add(pkg); err != nil {
			log.Fatalf("Error adding %s to graph: %v", pkg, err)
		}
	}

	// Listen for errors.
	for {
		select {
		case err := <-errCh:
			log.Printf("ERROR: %v", err)
		}
	}
}
