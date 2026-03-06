// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package engine

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

func TestScanErrorCollectorAddNilIsNoop(t *testing.T) {
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	collector := &scanErrorCollector{
		cancel: cancel,
		logger: logger,
	}

	collector.Add(nil)

	if err := collector.Err(); err != nil {
		t.Fatalf("expected nil error after Add(nil), got %v", err)
	}
}

func TestScanErrorCollectorSuppressesCanceledAfterFirstError(t *testing.T) {
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	collector := &scanErrorCollector{
		cancel: cancel,
		logger: logger,
	}

	first := errors.New("real scanner error")
	collector.Add(first)

	// context.Canceled after a real error should be suppressed.
	collector.Add(context.Canceled)
	// context.DeadlineExceeded after a real error should also be suppressed.
	collector.Add(context.DeadlineExceeded)

	err := collector.Err()
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if !errors.Is(err, first) {
		t.Fatal("expected error to contain the first real error")
	}
	if errors.Is(err, context.Canceled) {
		t.Fatal("expected context.Canceled to be suppressed")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("expected context.DeadlineExceeded to be suppressed")
	}
}
