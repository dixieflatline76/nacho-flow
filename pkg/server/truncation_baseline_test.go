// Copyright (c) 2026 Karl Kwong / Spicebox. Licensed under AGPL-3.0.
// SPDX-License-Identifier: AGPL-3.0-or-later

package server

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestTruncationBaseline_TruthfulBehavior asserts the truthful termination behavior
// of StreamNormalizer on truncation fixtures F1-F9.
func TestTruncationBaseline_TruthfulBehavior(t *testing.T) {
	fixtureDir := filepath.Join("testdata", "truncation")

	t.Run("F1_reasoning_length_truthful", func(t *testing.T) {
		raw, err := os.ReadFile(filepath.Join(fixtureDir, "F1_reasoning_length.sse"))
		if err != nil {
			t.Fatalf("failed to read fixture: %v", err)
		}

		norm := NewStreamNormalizer(io.NopCloser(bytes.NewReader(raw)))
		defer norm.Close()

		out, err := io.ReadAll(norm)
		if err != nil {
			t.Fatalf("read failed: %v", err)
		}
		result := string(out)

		// Verification of truthful termination: length preserved, no fake text, no double finish
		if !strings.Contains(result, `"finish_reason":"length"`) {
			t.Errorf("expected original length finish chunk")
		}
		if strings.Contains(result, "Review and codebase verification completed") {
			t.Errorf("expected NO fake resuscitation text in truthful output")
		}
		if strings.Contains(result, `"finish_reason":"stop"`) {
			t.Errorf("expected NO second finish_reason stop")
		}
		if !strings.Contains(result, "Model reached its output token limit without producing a response") {
			t.Errorf("expected honest notice chunk")
		}
	})

	t.Run("F2_reasoning_stop_empty", func(t *testing.T) {
		raw, err := os.ReadFile(filepath.Join(fixtureDir, "F2_reasoning_stop_empty.sse"))
		if err != nil {
			t.Fatalf("failed to read fixture: %v", err)
		}

		norm := NewStreamNormalizer(io.NopCloser(bytes.NewReader(raw)))
		defer norm.Close()

		out, err := io.ReadAll(norm)
		if err != nil {
			t.Fatalf("read failed: %v", err)
		}
		result := string(out)

		// Honest stop finish with zero fake resuscitation text
		if strings.Contains(result, "Review and codebase verification completed") {
			t.Errorf("expected NO resuscitation text on empty stop turn")
		}
		if !strings.Contains(result, `"finish_reason":"stop"`) {
			t.Errorf("expected finish_reason stop")
		}
		if count := strings.Count(result, `"finish_reason":`); count != 1 {
			t.Errorf("expected exactly 1 finish_reason chunk, got %d", count)
		}
	})

	t.Run("F4_partial_toolcall_length_no_fake_resuscitation", func(t *testing.T) {
		raw, err := os.ReadFile(filepath.Join(fixtureDir, "F4_partial_toolcall_length.sse"))
		if err != nil {
			t.Fatalf("failed to read fixture: %v", err)
		}

		norm := NewStreamNormalizer(io.NopCloser(bytes.NewReader(raw)))
		defer norm.Close()

		out, err := io.ReadAll(norm)
		if err != nil {
			t.Fatalf("read failed: %v", err)
		}
		result := string(out)

		// Has emitted tool call delta -> no resuscitation
		if strings.Contains(result, "Review and codebase verification completed") {
			t.Errorf("expected no resuscitation when tool calls were emitted")
		}
	})

	t.Run("F5_partial_prose_length_no_fake_resuscitation", func(t *testing.T) {
		raw, err := os.ReadFile(filepath.Join(fixtureDir, "F5_partial_prose_length.sse"))
		if err != nil {
			t.Fatalf("failed to read fixture: %v", err)
		}

		norm := NewStreamNormalizer(io.NopCloser(bytes.NewReader(raw)))
		defer norm.Close()

		out, err := io.ReadAll(norm)
		if err != nil {
			t.Fatalf("read failed: %v", err)
		}
		result := string(out)

		// Has emitted content -> no resuscitation
		if strings.Contains(result, "Review and codebase verification completed") {
			t.Errorf("expected no resuscitation when content was emitted")
		}
	})

	t.Run("F7_no_reasoning_length", func(t *testing.T) {
		raw, err := os.ReadFile(filepath.Join(fixtureDir, "F7_no_reasoning_length.sse"))
		if err != nil {
			t.Fatalf("failed to read fixture: %v", err)
		}

		norm := NewStreamNormalizer(io.NopCloser(bytes.NewReader(raw)))
		defer norm.Close()

		out, err := io.ReadAll(norm)
		if err != nil {
			t.Fatalf("read failed: %v", err)
		}
		result := string(out)

		// No reasoning emitted -> no resuscitation
		if strings.Contains(result, "Review and codebase verification completed") {
			t.Errorf("expected no resuscitation when no reasoning was emitted")
		}
		if !strings.Contains(result, `"finish_reason":"length"`) {
			t.Errorf("expected length finish chunk")
		}
	})
}
