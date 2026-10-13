package wirejson

import (
	"bytes"
	"encoding/json"
)

// Marshal returns the JSON encoding of v without HTML escaping (<, >, & are preserved)
// and without the trailing newline added by json.Encoder.
func Marshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	b := buf.Bytes()
	if len(b) > 0 && b[len(b)-1] == '\n' {
		b = b[:len(b)-1]
	}
	return b, nil
}
