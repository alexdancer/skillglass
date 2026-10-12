package terminal

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestYAMLSourceOffsetsUseRuneColumnsAndLineBoundaries(t *testing.T) {
	offsets, ok := yamlSourceOffsets("é😀a\r\n\n字z\n")
	if !ok {
		t.Fatal("LF and CRLF must be supported")
	}
	for _, test := range []struct {
		line, column, offset int
		valid                bool
	}{
		{1, 1, 0, true}, {1, 2, 2, true}, {1, 3, 6, true},
		{1, 4, 0, false}, {2, 1, 0, false},
		{3, 1, 10, true}, {3, 2, 13, true}, {3, 3, 0, false},
		{4, 1, 0, false}, {0, 1, 0, false}, {1, 0, 0, false}, {5, 1, 0, false},
	} {
		got, valid := yamlSourceOffset(offsets, test.line, test.column)
		if got != test.offset || valid != test.valid {
			t.Fatalf("(%d, %d) = (%d, %t), want (%d, %t)", test.line, test.column, got, valid, test.offset, test.valid)
		}
	}
	for _, boundary := range []string{"\r", "\u0085", "\u2028", "\u2029"} {
		if _, ok := yamlSourceOffsets("key: value" + boundary + "tail: value\n"); ok {
			t.Fatalf("unsupported boundary %q accepted", boundary)
		}
	}
}

func TestYAMLMappingKeySpansAcrossLargeUnicodeFlowLines(t *testing.T) {
	var raw strings.Builder
	var want []sourceSpan
	writeKey := func(key string) {
		start := raw.Len()
		raw.WriteString(key)
		want = append(want, sourceSpan{start: start, end: raw.Len()})
	}
	writeKey("name")
	raw.WriteString(": probe\r\n")
	for line := 0; line < 2; line++ {
		writeKey(fmt.Sprintf("flow%d", line))
		raw.WriteString(": {")
		for key := 0; key < 24000; key++ {
			if key > 0 {
				raw.WriteString(", ")
			}
			writeKey(fmt.Sprintf("\"clé😀:%d\"", key))
			raw.WriteString(": \"clé😀: neutral\"")
		}
		raw.WriteString("}\r\n")
	}
	got, ok := yamlMappingKeySpans(raw.String())
	if !ok || !reflect.DeepEqual(got, want) {
		t.Fatalf("large Unicode flow mapping produced incorrect key spans: valid=%t, got %d keys, want %d", ok, len(got), len(want))
	}
}
