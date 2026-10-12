// Copyright (c) 2026 Karl Kwong / Spicebox. Licensed under AGPL-3.0.
// SPDX-License-Identifier: AGPL-3.0-or-later

package server

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// TestStreamNormalizer_TruncationDetection verifies that finish_reason is tracked
// and IsEmptyTruncatedTurn correctly identifies empty truncated turns without changing any output bytes.
func TestStreamNormalizer_TruncationDetection(t *testing.T) {
	fixtureDir := filepath.Join("testdata", "truncation")

	t.Run("F1_reasoning_length is detected as empty truncated turn", func(t *testing.T) {
		raw, err := os.ReadFile(filepath.Join(fixtureDir, "F1_reasoning_length.sse"))
		if err != nil {
			t.Fatalf("failed to read fixture: %v", err)
		}

		norm := NewStreamNormalizer(io.NopCloser(bytes.NewReader(raw)))
		defer norm.Close()

		_, err = io.ReadAll(norm)
		if err != nil {
			t.Fatalf("read failed: %v", err)
		}

		if norm.FinishReason() != "length" {
			t.Errorf("expected FinishReason 'length', got %q", norm.FinishReason())
		}
		if !norm.IsEmptyTruncatedTurn() {
			t.Errorf("expected IsEmptyTruncatedTurn to be true for F1")
		}
	})

	t.Run("F2_reasoning_stop_empty is not empty truncated turn (stop != length)", func(t *testing.T) {
		raw, err := os.ReadFile(filepath.Join(fixtureDir, "F2_reasoning_stop_empty.sse"))
		if err != nil {
			t.Fatalf("failed to read fixture: %v", err)
		}

		norm := NewStreamNormalizer(io.NopCloser(bytes.NewReader(raw)))
		defer norm.Close()

		_, err = io.ReadAll(norm)
		if err != nil {
			t.Fatalf("read failed: %v", err)
		}

		if norm.FinishReason() != "stop" {
			t.Errorf("expected FinishReason 'stop', got %q", norm.FinishReason())
		}
		if norm.IsEmptyTruncatedTurn() {
			t.Errorf("expected IsEmptyTruncatedTurn to be false for stop finish")
		}
	})

	t.Run("F3_whitespace_content_length is detected as empty truncated turn", func(t *testing.T) {
		raw, err := os.ReadFile(filepath.Join(fixtureDir, "F3_whitespace_content_length.sse"))
		if err != nil {
			t.Fatalf("failed to read fixture: %v", err)
		}

		norm := NewStreamNormalizer(io.NopCloser(bytes.NewReader(raw)))
		defer norm.Close()

		_, err = io.ReadAll(norm)
		if err != nil {
			t.Fatalf("read failed: %v", err)
		}

		if norm.FinishReason() != "length" {
			t.Errorf("expected FinishReason 'length', got %q", norm.FinishReason())
		}
		if !norm.IsEmptyTruncatedTurn() {
			t.Errorf("expected IsEmptyTruncatedTurn to be true for whitespace content + length")
		}
	})

	t.Run("F4_partial_toolcall_length is not empty (has tool calls)", func(t *testing.T) {
		raw, err := os.ReadFile(filepath.Join(fixtureDir, "F4_partial_toolcall_length.sse"))
		if err != nil {
			t.Fatalf("failed to read fixture: %v", err)
		}

		norm := NewStreamNormalizer(io.NopCloser(bytes.NewReader(raw)))
		defer norm.Close()

		_, err = io.ReadAll(norm)
		if err != nil {
			t.Fatalf("read failed: %v", err)
		}

		if norm.FinishReason() != "length" {
			t.Errorf("expected FinishReason 'length', got %q", norm.FinishReason())
		}
		if norm.IsEmptyTruncatedTurn() {
			t.Errorf("expected IsEmptyTruncatedTurn to be false when tool calls were emitted")
		}
	})

	t.Run("F5_partial_prose_length is not empty (has non-whitespace content)", func(t *testing.T) {
		raw, err := os.ReadFile(filepath.Join(fixtureDir, "F5_partial_prose_length.sse"))
		if err != nil {
			t.Fatalf("failed to read fixture: %v", err)
		}

		norm := NewStreamNormalizer(io.NopCloser(bytes.NewReader(raw)))
		defer norm.Close()

		_, err = io.ReadAll(norm)
		if err != nil {
			t.Fatalf("read failed: %v", err)
		}

		if norm.FinishReason() != "length" {
			t.Errorf("expected FinishReason 'length', got %q", norm.FinishReason())
		}
		if norm.IsEmptyTruncatedTurn() {
			t.Errorf("expected IsEmptyTruncatedTurn to be false when content was emitted")
		}
	})

	t.Run("F7_no_reasoning_length is detected as empty truncated turn", func(t *testing.T) {
		raw, err := os.ReadFile(filepath.Join(fixtureDir, "F7_no_reasoning_length.sse"))
		if err != nil {
			t.Fatalf("failed to read fixture: %v", err)
		}

		norm := NewStreamNormalizer(io.NopCloser(bytes.NewReader(raw)))
		defer norm.Close()

		_, err = io.ReadAll(norm)
		if err != nil {
			t.Fatalf("read failed: %v", err)
		}

		if norm.FinishReason() != "length" {
			t.Errorf("expected FinishReason 'length', got %q", norm.FinishReason())
		}
		if !norm.IsEmptyTruncatedTurn() {
			t.Errorf("expected IsEmptyTruncatedTurn to be true for F7")
		}
	})
}
