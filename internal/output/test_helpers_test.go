// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package output

import "bytes"

func normalizeGoldenNewlines(data []byte) []byte {
	data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	data = bytes.ReplaceAll(data, []byte("\r"), []byte("\n"))
	return data
}
