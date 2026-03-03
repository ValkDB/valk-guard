// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

// Package schema provides types and builders for cross-referencing ORM model
// definitions against migration DDL. It accumulates a schema snapshot from
// parsed SQL statements and exposes model extraction interfaces used by
// language-specific extractors.
package schema
