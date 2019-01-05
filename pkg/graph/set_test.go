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

func TestSetOperations(t *testing.T) {
	set := make(StringSet)

	keys := []string{"aaa", "bbb", "ccc"}

	for _, key := range keys {
		if set.Has(key) {
			t.Errorf("Has(%s) = true, wanted false", key)
		}

		set.Add(key)
		set.Add(key) // Check idempotency

		if !set.Has(key) {
			t.Errorf("Has(%s) = false, wanted true", key)
		}
	}

	order := set.InOrder()
	if want, got := len(keys), len(order); got != want {
		t.Errorf("len(InOrder) = %d, wanted %d", got, want)
	}
	for i, got := range order {
		want := keys[i]

		if got != want {
			t.Errorf("InOrder[%d] = %s, wanted %s", i, got, want)
		}
	}

	for _, key := range keys {
		if !set.Has(key) {
			t.Errorf("Has(%s) = false, wanted true", key)
		}

		set.Remove(key)
		set.Remove(key) // Check idempotency

		if set.Has(key) {
			t.Errorf("Has(%s) = true, wanted false", key)
		}
	}
}
