// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
)

func TestScanErrorCollectorJoinsScannerErrors(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	collector := &scanErrorCollector{
		cancel: cancel,
		logger: logger,
	}

	first := errors.New("first scanner failed")
	second := errors.New("second scanner failed")

	collector.Add(first)
	if ctx.Err() == nil {
		t.Fatal("expected first scanner error to cancel the scan context")
	}

	collector.Add(second)

	err := collector.Err()
	if err == nil {
		t.Fatal("expected joined scanner error")
	}
	if !errors.Is(err, first) {
		t.Fatal("expected joined error to contain the first scanner error")
	}
	if !errors.Is(err, second) {
		t.Fatal("expected joined error to contain the second scanner error")
	}
}
