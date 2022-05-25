package resource

import (
	"fmt"
	"sort"
	"strings"
)

// stringify returns a formatted string (one k/v per line) of map xs.
func stringify[T any](xs map[string]T) string {
	// Sort the keys in alphabetical order.
	keys := make([]string, 0, len(xs))
	for k := range xs {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var bld strings.Builder

	for _, k := range keys {
		fmt.Fprintf(&bld, "  %s: %v\n", k, xs[k])
	}

	return bld.String()
}
