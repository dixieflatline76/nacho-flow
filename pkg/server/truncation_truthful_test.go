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

// TestStreamNormalizer_TruthfulTermination asserts the Phase 1b truthful termination invariants:
// 1. Never fabricate assistant verification prose
// 2. Exactly one finish_reason chunk per stream
// 3. Honest terminal notice emitted on empty length-truncated turns
func TestStreamNormalizer_TruthfulTermination(t *testing.T) {
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

		// 1. Fake resuscitation prose must be completely gone
		if strings.Contains(result, "Review and codebase verification completed") {
			t.Errorf("expected NO fake resuscitation text in truthful output, got:\n%s", result)
		}

		// 2. Must contain honest terminal notice
		if !strings.Contains(result, "Model reached its output token limit without producing a response") {
			t.Errorf("expected honest notice chunk, got:\n%s", result)
		}

		// 3. Must preserve original finish_reason: length
		if !strings.Contains(result, `"finish_reason":"length"`) {
			t.Errorf("expected length finish_reason, got:\n%s", result)
		}

		// 4. Must NOT contain a second finish_reason: stop
		if strings.Contains(result, `"finish_reason":"stop"`) {
			t.Errorf("did NOT expect finish_reason stop, got:\n%s", result)
		}

		// 5. Invariant: exactly one finish_reason in the entire stream
		if count := strings.Count(result, `"finish_reason":`); count != 1 {
			t.Errorf("expected exactly 1 finish_reason chunk, got %d", count)
		}

		// 6. Invariant: exactly one [DONE] marker
		if count := strings.Count(result, "data: [DONE]"); count != 1 {
			t.Errorf("expected exactly 1 [DONE] marker, got %d", count)
		}
	})

	t.Run("F2_reasoning_stop_empty_no_fabrication", func(t *testing.T) {
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

		// No fake resuscitation text
		if strings.Contains(result, "Review and codebase verification completed") {
			t.Errorf("expected NO fake resuscitation text on empty stop turn, got:\n%s", result)
		}

		// Honest stop finish
		if !strings.Contains(result, `"finish_reason":"stop"`) {
			t.Errorf("expected finish_reason stop, got:\n%s", result)
		}

		// Invariant: exactly one finish_reason
		if count := strings.Count(result, `"finish_reason":`); count != 1 {
			t.Errorf("expected exactly 1 finish_reason chunk, got %d", count)
		}
	})

	t.Run("F7_no_reasoning_length_truthful", func(t *testing.T) {
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

		if strings.Contains(result, "Review and codebase verification completed") {
			t.Errorf("expected NO fake resuscitation text, got:\n%s", result)
		}
		if !strings.Contains(result, "Model reached its output token limit without producing a response") {
			t.Errorf("expected honest notice chunk, got:\n%s", result)
		}
		if count := strings.Count(result, `"finish_reason":`); count != 1 {
			t.Errorf("expected exactly 1 finish_reason chunk, got %d", count)
		}
	})

	t.Run("F8_anthropic_max_tokens_truthful", func(t *testing.T) {
		raw, err := os.ReadFile(filepath.Join(fixtureDir, "F8_anthropic_max_tokens.sse"))
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

		if strings.Contains(result, "Review and codebase verification completed") {
			t.Errorf("expected NO fake resuscitation text, got:\n%s", result)
		}
		if !strings.Contains(result, "Model reached its output token limit without producing a response") {
			t.Errorf("expected honest notice chunk, got:\n%s", result)
		}
		if count := strings.Count(result, `"finish_reason":`); count != 1 {
			t.Errorf("expected exactly 1 finish_reason chunk, got %d", count)
		}
	})

	t.Run("F9_inline_think_length_truthful", func(t *testing.T) {
		raw, err := os.ReadFile(filepath.Join(fixtureDir, "F9_inline_think_length.sse"))
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

		if strings.Contains(result, "Review and codebase verification completed") {
			t.Errorf("expected NO fake resuscitation text, got:\n%s", result)
		}
		if !strings.Contains(result, "Model reached its output token limit without producing a response") {
			t.Errorf("expected honest notice chunk, got:\n%s", result)
		}
		if count := strings.Count(result, `"finish_reason":`); count != 1 {
			t.Errorf("expected exactly 1 finish_reason chunk, got %d", count)
		}
	})

	t.Run("All_Fixtures_Single_Finish_Single_Done_Invariant", func(t *testing.T) {
		files, err := os.ReadDir(fixtureDir)
		if err != nil {
			t.Fatalf("failed to read fixture directory: %v", err)
		}

		for _, f := range files {
			if !strings.HasSuffix(f.Name(), ".sse") {
				continue
			}
			t.Run(f.Name(), func(t *testing.T) {
				raw, err := os.ReadFile(filepath.Join(fixtureDir, f.Name()))
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

				// Invariant 1: Exactly 1 finish_reason chunk
				if count := strings.Count(result, `"finish_reason":`); count != 1 {
					t.Errorf("%s: expected exactly 1 finish_reason, got %d\nOutput:\n%s", f.Name(), count, result)
				}

				// Invariant 2: Exactly 1 [DONE] marker
				if count := strings.Count(result, "data: [DONE]"); count != 1 {
					t.Errorf("%s: expected exactly 1 [DONE] marker, got %d\nOutput:\n%s", f.Name(), count, result)
				}

				// Invariant 3: Zero fake resuscitation text
				if strings.Contains(result, "Review and codebase verification completed") {
					t.Errorf("%s: unexpected fake resuscitation text found in output", f.Name())
				}
			})
		}
	})
}
