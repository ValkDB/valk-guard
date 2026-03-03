// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package textutil

import "testing"

func TestContainsFoldTrim(t *testing.T) {
	t.Parallel()

	flags := []string{" concurrently ", "if_not_exists", "ADD_COLUMN"}
	if !ContainsFoldTrim(flags, "CONCURRENTLY") {
		t.Fatal("expected match for concurrently")
	}
	if !ContainsFoldTrim(flags, "add_column") {
		t.Fatal("expected case-insensitive match for add_column")
	}
	if ContainsFoldTrim(flags, "missing") {
		t.Fatal("did not expect match for missing")
	}
}
