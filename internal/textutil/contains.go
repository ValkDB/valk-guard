// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package textutil

import "strings"

// ContainsFoldTrim reports whether values contains want, comparing each value
// case-insensitively after trimming surrounding whitespace.
func ContainsFoldTrim(values []string, want string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), want) {
			return true
		}
	}
	return false
}
