// Copyright 2019 The Go Cloud Development Kit Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package driver

import (
	"reflect"
	"sort"

	"github.com/google/uuid"
)

// UniqueString generates a string that is unique with high probability.
// Driver implementations can use it to generate keys for Create actions.
func UniqueString() string { return uuid.New().String() }

// SplitActions divides the actions slice into sub-slices much like strings.Split.
// The split function should report whether two consecutive actions should be split,
// that is, should be in different sub-slices. The first argument to split is the
// last action of the sub-slice currently under construction; the second argument is
// the action being considered for addition to that sub-slice.
// SplitActions doesn't change the order of the input slice.
func SplitActions(actions []*Action, split func(a, b *Action) bool) [][]*Action {
	var (
		groups [][]*Action // the actions, split; the return value
		cur    []*Action   // the group currently being constructed
	)
	collect := func() { // called when the current group is known to be finished
		if len(cur) > 0 {
			groups = append(groups, cur)
			cur = nil
		}
	}
	for _, a := range actions {
		if len(cur) > 0 && split(cur[len(cur)-1], a) {
			collect()
		}
		cur = append(cur, a)
	}
	collect()
	return groups
}

// GroupActions separates actions into four sets: writes, gets that must happen before the writes,
// gets that must happen after the writes, and gets that can happen concurrently with the writes.
func GroupActions(actions []*Action) (beforeGets, getList, writeList, afterGets []*Action) {
	// maps from key to action
	bgets := map[interface{}]*Action{}
	agets := map[interface{}]*Action{}
	cgets := map[interface{}]*Action{}
	writes := map[interface{}]*Action{}
	for _, a := range actions {
		if a.Kind == Get {
			// If there was a prior write with this key, make sure this get
			// happens after the writes.
			if _, ok := writes[a.Key]; ok {
				agets[a.Key] = a
			} else {
				cgets[a.Key] = a
			}
		} else {
			// This is a write. A prior get on the same key was put into cgets; move
			// it to bgets because it has to happen before writes.
			if g, ok := cgets[a.Key]; ok {
				delete(cgets, a.Key)
				bgets[a.Key] = g
			}
			writes[a.Key] = a
		}
	}

	vals := func(m map[interface{}]*Action) []*Action {
		var as []*Action
		for _, v := range m {
			as = append(as, v)
		}
		// Sort so the order is always the same for replay.
		sort.Slice(as, func(i, j int) bool { return as[i].Index < as[j].Index })
		return as
	}

	return vals(bgets), vals(cgets), vals(writes), vals(agets)
}

// AsFunc creates and returns an "as function" that behaves as follows:
// If its argument is a pointer to the same type as val, the argument is set to val
// and the function returns true. Otherwise, the function returns false.
func AsFunc(val interface{}) func(interface{}) bool {
	rval := reflect.ValueOf(val)
	wantType := reflect.PtrTo(rval.Type())
	return func(i interface{}) bool {
		if i == nil {
			return false
		}
		ri := reflect.ValueOf(i)
		if ri.Type() != wantType {
			return false
		}
		ri.Elem().Set(rval)
		return true
	}
}

// GroupByFieldPath collect the Get actions into groups with the same set of
// field paths.
func GroupByFieldPath(gets []*Action) [][]*Action {
	// This is quadratic in the worst case, but it's unlikely that there would be
	// many Gets with different field paths.
	var groups [][]*Action
	seen := map[*Action]bool{}
	for len(seen) < len(gets) {
		var g []*Action
		for _, a := range gets {
			if !seen[a] {
				if len(g) == 0 || fpsEqual(g[0].FieldPaths, a.FieldPaths) {
					g = append(g, a)
					seen[a] = true
				}
			}
		}
		groups = append(groups, g)
	}
	return groups
}

// Report whether two lists of field paths are equal.
func fpsEqual(fps1, fps2 [][]string) bool {
	// TODO?: We really care about sets of field paths, but that's too tedious to determine.
	if len(fps1) != len(fps2) {
		return false
	}
	for i, fp1 := range fps1 {
		if !fpEqual(fp1, fps2[i]) {
			return false
		}
	}
	return true
}

// Report whether two field paths are equal.
func fpEqual(fp1, fp2 []string) bool {
	if len(fp1) != len(fp2) {
		return false
	}
	for i, s1 := range fp1 {
		if s1 != fp2[i] {
			return false
		}
	}
	return true
}
