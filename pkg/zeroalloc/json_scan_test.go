package zeroalloc

import (
	"bytes"
	"encoding/json"
	"testing"
)

func findSpan(b []byte, sub string) (int, int) {
	idx := bytes.Index(b, []byte(sub))
	if idx == -1 {
		panic("sub not found: " + sub)
	}
	return idx, idx + len(sub)
}

func TestSkipValue_Primitives(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{"integer", "12345"},
		{"negative float exp", "-123.45e-6"},
		{"true", "true"},
		{"false", "false"},
		{"null", "null"},
		{"empty string", `""`},
		{"simple string", `"hello world"`},
		{"string with escapes", `"hello \"escaped\" \\ \/ \b \f \n \r \t \u1234"`},
		{"string with braces and brackets", `"{} [] , :"`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := []byte(tc.in)
			end, ok := SkipValue(b, 0)
			if !ok {
				t.Fatalf("SkipValue failed on %q", tc.in)
			}
			if end != len(tc.in) {
				t.Fatalf("SkipValue(%q) = %d, want %d", tc.in, end, len(tc.in))
			}
		})
	}
}

func TestSkipValue_LeadingWhitespace(t *testing.T) {
	in := []byte("  \t\r\n  {\"key\": 123}")
	end, ok := SkipValue(in, 0)
	if !ok {
		t.Fatalf("SkipValue failed on leading whitespace")
	}
	if end != len(in) {
		t.Fatalf("SkipValue got %d, want %d", end, len(in))
	}
}

func TestSkipValue_Containers(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{"empty object", "{}"},
		{"empty array", "[]"},
		{"simple object", `{"a": 1, "b": "two"}`},
		{"nested object", `{"a": {"b": [1, 2, "}"], "c": "{\"d\": 4}"}}`},
		{"array with objects and strings", `[1, "foo", [true, false], {"x": "]"}]`},
		{"trailing comma context", `{"a": [1, 2]}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := []byte(tc.in)
			end, ok := SkipValue(b, 0)
			if !ok {
				t.Fatalf("SkipValue failed on %q", tc.in)
			}
			if end != len(tc.in) {
				t.Fatalf("SkipValue(%q) = %d, want %d", tc.in, end, len(tc.in))
			}
		})
	}
}

func TestSkipValue_Invalid(t *testing.T) {
	invalids := []string{
		"",
		"   ",
		`"unterminated string`,
		`"escaped quote at end \"`,
		`{`,
		`[`,
		`{"a": `,
		`{"a": 1`,
		`[1, 2`,
		`}`,
		`]`,
		`:`,
		`,`,
		`tru`,
		`fals`,
		`nul`,
	}

	for _, inv := range invalids {
		t.Run(inv, func(t *testing.T) {
			end, ok := SkipValue([]byte(inv), 0)
			if ok {
				t.Errorf("SkipValue(%q) expected ok=false, got end=%d", inv, end)
			}
		})
	}
}

func TestObjectField(t *testing.T) {
	jsonDoc := []byte(`{
		"model": "gpt-4o",
		"messages": [{"role": "user", "content": "hello"}],
		"stream": false,
		"deep": {"model": "inner", "val": 123},
		"str_with_keys": "not a \"model\": dummy",
		"last": true
	}`)

	// Match "model" at top level (must find the top-level one, not the deep one)
	keyStart, valStart, valEnd, ok := ObjectField(jsonDoc, 0, len(jsonDoc), "model")
	if !ok {
		t.Fatalf("ObjectField('model') not found")
	}
	if string(jsonDoc[valStart:valEnd]) != `"gpt-4o"` {
		t.Errorf("expected 'gpt-4o', got %s", string(jsonDoc[valStart:valEnd]))
	}
	if !bytes.Contains(jsonDoc[keyStart:valStart], []byte(`"model"`)) {
		t.Errorf("keyStart incorrect: %s", string(jsonDoc[keyStart:valStart]))
	}

	// Match "stream"
	_, valStart, valEnd, ok = ObjectField(jsonDoc, 0, len(jsonDoc), "stream")
	if !ok || string(jsonDoc[valStart:valEnd]) != "false" {
		t.Errorf("ObjectField('stream') failed: ok=%v, val=%s", ok, string(jsonDoc[valStart:valEnd]))
	}

	// Match "deep"
	_, valStart, valEnd, ok = ObjectField(jsonDoc, 0, len(jsonDoc), "deep")
	if !ok || !bytes.HasPrefix(jsonDoc[valStart:valEnd], []byte(`{"model"`)) {
		t.Errorf("ObjectField('deep') failed: ok=%v, val=%s", ok, string(jsonDoc[valStart:valEnd]))
	}

	// Match inside sub-object using the sub-object bounds
	_, innerStart, innerEnd, ok := ObjectField(jsonDoc, valStart, valEnd, "model")
	if !ok || string(jsonDoc[innerStart:innerEnd]) != `"inner"` {
		t.Errorf("ObjectField inside 'deep' failed: ok=%v, val=%s", ok, string(jsonDoc[innerStart:innerEnd]))
	}

	// Search for non-existent key
	_, _, _, ok = ObjectField(jsonDoc, 0, len(jsonDoc), "nonexistent")
	if ok {
		t.Errorf("ObjectField('nonexistent') expected ok=false, got true")
	}

	// Invalid bounds
	_, _, _, ok = ObjectField(jsonDoc, 5, 10, "model")
	if ok {
		t.Errorf("ObjectField on non-object bounds expected ok=false")
	}
}

func TestNextElement(t *testing.T) {
	arrDoc := []byte(`[123, "hello, world", {"a": [1, 2]}, [3, 4], true]`)
	pos := 1 // right after '['
	var elems []string

	for {
		start, end, next, ok := NextElement(arrDoc, pos, len(arrDoc))
		if !ok {
			break
		}
		elems = append(elems, string(arrDoc[start:end]))
		pos = next
	}

	expected := []string{
		"123",
		`"hello, world"`,
		`{"a": [1, 2]}`,
		`[3, 4]`,
		"true",
	}

	if len(elems) != len(expected) {
		t.Fatalf("NextElement got %d elements, want %d: %v", len(elems), len(expected), elems)
	}
	for i := range expected {
		if elems[i] != expected[i] {
			t.Errorf("elem %d = %q, want %q", i, elems[i], expected[i])
		}
	}

	// Empty array
	emptyArr := []byte(`[]`)
	_, _, _, ok := NextElement(emptyArr, 1, len(emptyArr))
	if ok {
		t.Errorf("NextElement on empty array expected ok=false")
	}

	// Whitespace array
	wsArr := []byte(`[   ]`)
	_, _, _, ok = NextElement(wsArr, 1, len(wsArr))
	if ok {
		t.Errorf("NextElement on whitespace empty array expected ok=false")
	}
}

func TestSpliceInPlace_AllCases(t *testing.T) {
	// 1. Shrink replacements
	src := []byte(`{"name":"alphabeta","age":42,"city":"san francisco"}`)
	s1, e1 := findSpan(src, `"alphabeta"`)
	s2, e2 := findSpan(src, `"san francisco"`)
	edits := []Edit{
		{Start: s1, End: e1, Repl: []byte(`"a"`)},  // shrink
		{Start: s2, End: e2, Repl: []byte(`"ny"`)}, // shrink
	}
	orig := append([]byte(nil), src...)
	out, ok := SpliceInPlace(src, edits)
	if !ok {
		t.Fatalf("SpliceInPlace returned ok=false")
	}
	want := `{"name":"a","age":42,"city":"ny"}`
	if string(out) != want {
		t.Errorf("SpliceInPlace = %s, want %s", string(out), want)
	}
	if !json.Valid(out) {
		t.Errorf("SpliceInPlace output is not valid JSON: %s", string(out))
	}

	// 2. Named edge-case tests
	// a. Edit at offset 0
	buf0 := []byte(`[1, 2, 3]`)
	edits0 := []Edit{{Start: 0, End: 2, Repl: []byte(`[` /* replaces "[1" with "[" */)}}
	out0, ok := SpliceInPlace(buf0, edits0)
	if !ok || string(out0) != `[, 2, 3]` {
		t.Errorf("Edit at offset 0 failed: ok=%v, out=%s", ok, string(out0))
	}

	// b. Edit ending at len(b)
	bufEnd := []byte(`[1, 2, 3456]`)
	sEnd, eEnd := findSpan(bufEnd, `3456]`)
	editsEnd := []Edit{{Start: sEnd, End: eEnd, Repl: []byte(`9]`)}}
	outEnd, ok := SpliceInPlace(bufEnd, editsEnd)
	if !ok || string(outEnd) != `[1, 2, 9]` {
		t.Errorf("Edit at end failed: ok=%v, out=%s", ok, string(outEnd))
	}

	// c. Adjacent edits
	bufAdj := []byte(`12345678`)
	editsAdj := []Edit{
		{Start: 0, End: 4, Repl: []byte(`ab`)},
		{Start: 4, End: 8, Repl: []byte(`cd`)},
	}
	outAdj, ok := SpliceInPlace(bufAdj, editsAdj)
	if !ok || string(outAdj) != `abcd` {
		t.Errorf("Adjacent edits failed: ok=%v, out=%s", ok, string(outAdj))
	}

	// d. Same-length replacement
	bufSame := []byte(`{"a": 123}`)
	sSame, eSame := findSpan(bufSame, `123`)
	editsSame := []Edit{{Start: sSame, End: eSame, Repl: []byte(`999`)}}
	outSame, ok := SpliceInPlace(bufSame, editsSame)
	if !ok || string(outSame) != `{"a": 999}` {
		t.Errorf("Same-length edit failed: ok=%v, out=%s", ok, string(outSame))
	}

	// e. Empty replacement (deletion)
	bufDel := []byte(`{"a": 1, "delete_me": true}`)
	sDel, eDel := findSpan(bufDel, `, "delete_me": true`)
	editsDel := []Edit{{Start: sDel, End: eDel, Repl: []byte{}}}
	outDel, ok := SpliceInPlace(bufDel, editsDel)
	if !ok || string(outDel) != `{"a": 1}` {
		t.Errorf("Empty replacement failed: ok=%v, out=%s", ok, string(outDel))
	}

	// f. No edits
	bufNone := []byte(`{"unchanged": true}`)
	outNone, ok := SpliceInPlace(bufNone, nil)
	if !ok || string(outNone) != `{"unchanged": true}` {
		t.Errorf("No edits failed")
	}

	// 3. Validation failure cases (original buffer MUST be untouched)
	valTests := []struct {
		name  string
		edits []Edit
	}{
		{"Start < 0", []Edit{{Start: -1, End: 5, Repl: []byte(`a`)}}},
		{"End > len", []Edit{{Start: 0, End: 100, Repl: []byte(`a`)}}},
		{"Start > End", []Edit{{Start: 5, End: 2, Repl: []byte(`a`)}}},
		{"Unsorted edits", []Edit{
			{Start: 10, End: 15, Repl: []byte(`a`)},
			{Start: 5, End: 8, Repl: []byte(`b`)},
		}},
		{"Overlapping edits", []Edit{
			{Start: 5, End: 12, Repl: []byte(`a`)},
			{Start: 10, End: 15, Repl: []byte(`b`)},
		}},
		{"Expansion attempted", []Edit{
			{Start: 5, End: 6, Repl: []byte(`longer_than_span`)},
		}},
	}

	for _, vt := range valTests {
		t.Run("Validation_"+vt.name, func(t *testing.T) {
			b := append([]byte(nil), orig...)
			out, ok := SpliceInPlace(b, vt.edits)
			if ok {
				t.Fatalf("expected ok=false for %s", vt.name)
			}
			if !bytes.Equal(b, orig) {
				t.Fatalf("buffer was mutated on validation failure for %s", vt.name)
			}
			if out != nil {
				t.Fatalf("expected out=nil on failure, got %s", string(out))
			}
		})
	}
}

func TestSpliceInto_AllCases(t *testing.T) {
	src := []byte(`{"name":"alphabeta","age":42,"city":"san francisco"}`)
	sName, eName := findSpan(src, `"alphabeta"`)
	sAge, eAge := findSpan(src, `42`)
	sCity, eCity := findSpan(src, `"san francisco"`)

	edits := []Edit{
		{Start: sName, End: eName, Repl: []byte(`"a"`)},                 // shrink
		{Start: sAge, End: eAge, Repl: []byte(`100`)},                   // expand (2 -> 3)
		{Start: sCity, End: eCity, Repl: []byte(`"san francisco bay"`)}, // expand (15 -> 19)
	}

	dst := make([]byte, 0, 128)
	out, ok := SpliceInto(dst, src, edits)
	if !ok {
		t.Fatalf("SpliceInto failed")
	}
	want := `{"name":"a","age":100,"city":"san francisco bay"}`
	if string(out) != want {
		t.Errorf("SpliceInto = %s, want %s", string(out), want)
	}
	if !json.Valid(out) {
		t.Errorf("SpliceInto output invalid JSON: %s", string(out))
	}

	// Validation failures
	_, ok = SpliceInto(nil, src, []Edit{{Start: 10, End: 5, Repl: []byte(`a`)}})
	if ok {
		t.Errorf("expected failure on Start > End")
	}
	_, ok = SpliceInto(nil, src, []Edit{{Start: -1, End: 5, Repl: []byte(`a`)}})
	if ok {
		t.Errorf("expected failure on Start < 0")
	}
	_, ok = SpliceInto(nil, src, []Edit{{Start: 0, End: 500, Repl: []byte(`a`)}})
	if ok {
		t.Errorf("expected failure on End > len(src)")
	}
}

func TestSpliceInPlace_Matches_SpliceInto(t *testing.T) {
	src := []byte(`{"items":[100, 200, 300, 400], "status": "active"}`)
	s1, e1 := findSpan(src, `100`)
	s2, e2 := findSpan(src, `200`)
	s3, e3 := findSpan(src, `"active"`)

	edits := []Edit{
		{Start: s1, End: e1, Repl: []byte(`1`)},      // shrink
		{Start: s2, End: e2, Repl: []byte(`22`)},     // shrink
		{Start: s3, End: e3, Repl: []byte(`"done"`)}, // shrink
	}

	intoOut, ok := SpliceInto(nil, src, edits)
	if !ok {
		t.Fatalf("SpliceInto failed")
	}

	inPlaceSrc := append([]byte(nil), src...)
	inPlaceOut, ok := SpliceInPlace(inPlaceSrc, edits)
	if !ok {
		t.Fatalf("SpliceInPlace failed")
	}

	if !bytes.Equal(intoOut, inPlaceOut) {
		t.Errorf("SpliceInPlace (%s) != SpliceInto (%s)", string(inPlaceOut), string(intoOut))
	}
}

func TestScanner_ZeroAlloc(t *testing.T) {
	src := []byte(`{"model": "gpt-4o", "messages": [{"role": "user", "content": "hi"}], "tools": [{"name": "bash"}]}`)

	// SkipValue allocs
	allocs := testing.AllocsPerRun(1000, func() {
		_, _ = SkipValue(src, 0)
	})
	if allocs != 0 {
		t.Errorf("SkipValue allocated %f heap objects, expected 0", allocs)
	}

	// ObjectField allocs
	allocs = testing.AllocsPerRun(1000, func() {
		_, _, _, _ = ObjectField(src, 0, len(src), "messages")
	})
	if allocs != 0 {
		t.Errorf("ObjectField allocated %f heap objects, expected 0", allocs)
	}

	// NextElement allocs
	_, valStart, valEnd, _ := ObjectField(src, 0, len(src), "messages")
	allocs = testing.AllocsPerRun(1000, func() {
		_, _, _, _ = NextElement(src, valStart+1, valEnd)
	})
	if allocs != 0 {
		t.Errorf("NextElement allocated %f heap objects, expected 0", allocs)
	}

	// SpliceInPlace allocs
	scratch := make([]byte, len(src))
	sModel, eModel := findSpan(src, `"gpt-4o"`)
	edits := []Edit{
		{Start: sModel, End: eModel, Repl: []byte(`"gpt"`)},
	}
	allocs = testing.AllocsPerRun(1000, func() {
		copy(scratch, src)
		_, _ = SpliceInPlace(scratch, edits)
	})
	if allocs != 0 {
		t.Errorf("SpliceInPlace allocated %f heap objects, expected 0", allocs)
	}
}

func FuzzSkipValue(f *testing.F) {
	seeds := []string{
		`{"a": 1, "b": "hello", "c": [1, 2, true, null]}`,
		`[{"role": "user", "content": "hello world"}]`,
		`"simple string"`,
		`12345.678`,
		`true`,
		`false`,
		`null`,
		`{}`,
		`[]`,
	}
	for _, s := range seeds {
		f.Add([]byte(s))
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		if !json.Valid(data) {
			return
		}
		// Find first non-whitespace
		start := 0
		for start < len(data) && (data[start] == ' ' || data[start] == '\t' || data[start] == '\n' || data[start] == '\r') {
			start++
		}
		if start == len(data) {
			return
		}

		end, ok := SkipValue(data, start)
		if !ok {
			t.Fatalf("valid JSON rejected by SkipValue: %q", string(data[start:]))
		}
		// Everything after end should only be whitespace for valid single JSON value
		for k := end; k < len(data); k++ {
			if data[k] != ' ' && data[k] != '\t' && data[k] != '\n' && data[k] != '\r' {
				t.Fatalf("trailing non-whitespace after end offset %d in %q", end, string(data))
			}
		}
	})
}

func FuzzObjectField(f *testing.F) {
	seeds := []string{
		`{"model": "gpt-4o", "messages": [], "stream": false}`,
		`{"a": 1, "b": 2, "c": 3}`,
		`{"seed": 1234567890123456789}`,
		`{"empty": {}}`,
	}
	for _, s := range seeds {
		f.Add([]byte(s))
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		if !json.Valid(data) {
			return
		}
		var m map[string]json.RawMessage
		if err := json.Unmarshal(data, &m); err != nil {
			return
		}

		for k, wantVal := range m {
			_, valStart, valEnd, ok := ObjectField(data, 0, len(data), k)
			if !ok {
				t.Fatalf("key %q found by json.Unmarshal but missed by ObjectField in %s", k, string(data))
			}
			// Both should represent semantically equal JSON values
			var v1, v2 interface{}
			_ = json.Unmarshal(wantVal, &v1)
			_ = json.Unmarshal(data[valStart:valEnd], &v2)
			b1, _ := json.Marshal(v1)
			b2, _ := json.Marshal(v2)
			if !bytes.Equal(b1, b2) {
				t.Fatalf("key %q value mismatch: json.Unmarshal=%s, ObjectField=%s", k, string(b1), string(b2))
			}
		}
	})
}

func FuzzSplice(f *testing.F) {
	seeds := []string{
		`{"model": "gpt-4o", "stream": true}`,
		`[1, 2, 3, 4]`,
		`{"a": "hello world"}`,
	}
	for _, s := range seeds {
		f.Add([]byte(s))
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		if !json.Valid(data) {
			return
		}
		// If object with "model", replace it
		_, valStart, valEnd, ok := ObjectField(data, 0, len(data), "model")
		if !ok {
			return
		}
		repl := []byte(`"claude"`)
		edits := []Edit{{Start: valStart, End: valEnd, Repl: repl}}

		// SpliceInto
		intoOut, okInto := SpliceInto(nil, data, edits)
		if !okInto {
			t.Fatalf("SpliceInto failed on valid edit")
		}
		if !json.Valid(intoOut) {
			t.Fatalf("SpliceInto produced invalid JSON: %s", string(intoOut))
		}

		// If replacement fits, test SpliceInPlace
		if len(repl) <= valEnd-valStart {
			inPlaceBuf := append([]byte(nil), data...)
			inPlaceOut, okPlace := SpliceInPlace(inPlaceBuf, edits)
			if !okPlace {
				t.Fatalf("SpliceInPlace failed when repl fits")
			}
			if !bytes.Equal(intoOut, inPlaceOut) {
				t.Fatalf("SpliceInPlace != SpliceInto: %s vs %s", string(inPlaceOut), string(intoOut))
			}
		}
	})
}

func TestScanner_BranchCoverage(t *testing.T) {
	// 1. skipString edge branches
	if _, ok := skipString([]byte(`"has control `+"\x01"+` char"`), 0); ok {
		t.Errorf("expected failure for unescaped control char")
	}
	if _, ok := skipString([]byte(`not a quote`), 0); ok {
		t.Errorf("expected failure for non-quote start")
	}
	if _, ok := skipString([]byte(`"`), 0); ok {
		t.Errorf("expected failure for single quote")
	}
	if _, ok := skipString([]byte(`"ends with backslash\`), 0); ok {
		t.Errorf("expected failure for trailing backslash")
	}

	// 2. skipBracketContainer edge branches
	if _, ok := skipBracketContainer([]byte(`not bracket`), 0); ok {
		t.Errorf("expected failure on non-bracket start")
	}
	if _, ok := skipBracketContainer([]byte(`{"a": 1]}`), 0); ok {
		t.Errorf("expected failure on mismatched bracket ']' for '{'")
	}
	if _, ok := skipBracketContainer([]byte(`[1, 2}`), 0); ok {
		t.Errorf("expected failure on mismatched bracket '}' for '['")
	}
	if _, ok := skipBracketContainer([]byte(`{"unclosed": "str`), 0); ok {
		t.Errorf("expected failure on unclosed string in container")
	}
	// Max nesting depth exceeded (>128)
	deepBuf := bytes.Repeat([]byte(`[`), 130)
	deepBuf = append(deepBuf, bytes.Repeat([]byte(`]`), 130)...)
	if _, ok := skipBracketContainer(deepBuf, 0); ok {
		t.Errorf("expected failure on exceeding max stack depth 128")
	}

	// 3. skipNumber edge branches
	if _, ok := skipNumber([]byte(`-`), 0); ok {
		t.Errorf("expected failure on bare '-'")
	}
	if _, ok := skipNumber([]byte(`1.`), 0); ok {
		t.Errorf("expected failure on '1.' without fraction")
	}
	if _, ok := skipNumber([]byte(`1e`), 0); ok {
		t.Errorf("expected failure on '1e' without exponent")
	}
	if _, ok := skipNumber([]byte(`1e+`), 0); ok {
		t.Errorf("expected failure on '1e+' without exponent")
	}
	if _, ok := skipNumber([]byte(`123abc`), 0); ok {
		t.Errorf("expected failure on number with trailing non-delimiter")
	}

	// 4. ObjectField edge branches
	doc := []byte(`{"a": 1, "b": 2}`)
	if _, _, _, ok := ObjectField(doc, -1, len(doc), "a"); ok {
		t.Errorf("expected failure on objStart < 0")
	}
	if _, _, _, ok := ObjectField(doc, 0, len(doc)+10, "a"); ok {
		t.Errorf("expected failure on objEnd > len")
	}
	if _, _, _, ok := ObjectField(doc, 5, 2, "a"); ok {
		t.Errorf("expected failure on objStart >= objEnd")
	}
	if _, _, _, ok := ObjectField([]byte(`[1, 2]`), 0, 6, "a"); ok {
		t.Errorf("expected failure on non-object")
	}
	if _, _, _, ok := ObjectField([]byte(`{not_quoted: 1}`), 0, 15, "a"); ok {
		t.Errorf("expected failure on unquoted key")
	}
	if _, _, _, ok := ObjectField([]byte(`{"key" 1}`), 0, 9, "key"); ok {
		t.Errorf("expected failure on missing colon")
	}
	if _, _, _, ok := ObjectField([]byte(`{"key": `), 0, 8, "key"); ok {
		t.Errorf("expected failure on EOF after colon")
	}
	if _, _, _, ok := ObjectField([]byte(`{"key": invalid}`), 0, 16, "key"); ok {
		t.Errorf("expected failure on invalid value")
	}
	if _, _, _, ok := ObjectField([]byte(`{`), 0, 1, "k"); ok {
		t.Errorf("expected failure on unclosed empty-like object")
	}

	// 5. NextElement edge branches
	arr := []byte(`[1, 2]`)
	if _, _, _, ok := NextElement(arr, -1, len(arr)); ok {
		t.Errorf("expected failure on pos < 0")
	}
	if _, _, _, ok := NextElement(arr, 0, len(arr)+10); ok {
		t.Errorf("expected failure on arrEnd > len")
	}
	if _, _, _, ok := NextElement(arr, 5, 2); ok {
		t.Errorf("expected failure on pos >= arrEnd")
	}
	if _, _, _, ok := NextElement([]byte(`[1, ]`), 3, 5); ok {
		t.Errorf("expected failure on trailing comma before ']'")
	}
	if _, _, _, ok := NextElement([]byte(`[invalid]`), 1, 9); ok {
		t.Errorf("expected failure on invalid array element")
	}

	// 6. SpliceInto unsorted edits
	if _, ok := SpliceInto(nil, []byte(`12345`), []Edit{{Start: 3, End: 4}, {Start: 1, End: 2}}); ok {
		t.Errorf("expected failure on unsorted edits in SpliceInto")
	}
}
