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

// redact makes a copy of dirty and, for each k/v pair of it, if k matches one of the
// keys in secrets AND v is a string, it redacts v. It returns the redacted copy.
// WARNING: it is able to redact only string values!
func redact(dirty map[string]any, secrets map[string]struct{}) map[string]any {
	clean := make(map[string]any, len(dirty))

	for k, v := range dirty {
		clean[k] = v

		if _, found := secrets[k]; !found {
			continue
		}

		// Attempt to redact.
		if s, ok := v.(string); ok && len(s) > 0 {
			clean[k] = "REDACTED"
		}
	}

	return clean
}
