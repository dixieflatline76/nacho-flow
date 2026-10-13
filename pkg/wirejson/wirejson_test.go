package wirejson

import (
	"bytes"
	"encoding/json"
	"math"
	"testing"
)

type rawMessageContainer struct {
	Name   string          `json:"name"`
	Schema json.RawMessage `json:"schema"`
}

func TestMarshal_NoHTMLEscaping(t *testing.T) {
	input := map[string]string{
		"text": "<script>alert('hello' & 'world');</script>",
		"comp": "a < b && c > d",
	}

	got, err := Marshal(input)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	// Must NOT contain \u003c, \u003e, \u0026
	if bytes.Contains(got, []byte(`\u003c`)) || bytes.Contains(got, []byte(`\u003e`)) || bytes.Contains(got, []byte(`\u0026`)) {
		t.Errorf("Marshal output contains HTML escape sequences: %s", string(got))
	}

	// Must contain literal <, >, &
	if !bytes.Contains(got, []byte("<script>")) || !bytes.Contains(got, []byte("c > d")) || !bytes.Contains(got, []byte("&&")) {
		t.Errorf("Marshal output missing expected literal HTML characters: %s", string(got))
	}
}

func TestMarshal_NoTrailingNewline(t *testing.T) {
	data := map[string]int{"a": 1, "b": 2}
	got, err := Marshal(data)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	if len(got) == 0 {
		t.Fatalf("expected non-empty output")
	}
	if got[len(got)-1] == '\n' {
		t.Errorf("Marshal output ends with trailing newline: %q", got)
	}
}

func TestMarshal_RawMessagePreserved(t *testing.T) {
	// Standard json.Marshal HTML-escapes inside json.RawMessage! wirejson.Marshal must not.
	c := rawMessageContainer{
		Name:   "my_tool",
		Schema: json.RawMessage(`{"description":"A < B & C > D","type":"object"}`),
	}

	got, err := Marshal(c)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	if bytes.Contains(got, []byte(`\u003c`)) || bytes.Contains(got, []byte(`\u003e`)) || bytes.Contains(got, []byte(`\u0026`)) {
		t.Errorf("Marshal escaped characters inside RawMessage: %s", string(got))
	}
	if !bytes.Contains(got, []byte("A < B & C > D")) {
		t.Errorf("Marshal did not preserve literal characters in RawMessage: %s", string(got))
	}
}

func TestMarshal_EmptyAndNil(t *testing.T) {
	got, err := Marshal(nil)
	if err != nil {
		t.Fatalf("Marshal(nil) error: %v", err)
	}
	if string(got) != "null" {
		t.Errorf("Marshal(nil) = %s, want null", string(got))
	}

	got, err = Marshal("")
	if err != nil {
		t.Fatalf("Marshal(\"\") error: %v", err)
	}
	if string(got) != `""` {
		t.Errorf("Marshal(\"\") = %s, want \"\"", string(got))
	}
}

func TestMarshal_Error(t *testing.T) {
	// math.NaN cannot be encoded in standard JSON
	_, err := Marshal(math.NaN())
	if err == nil {
		t.Fatalf("expected error for math.NaN, got nil")
	}

	// channel cannot be encoded
	ch := make(chan int)
	_, err = Marshal(ch)
	if err == nil {
		t.Fatalf("expected error for channel, got nil")
	}
}
