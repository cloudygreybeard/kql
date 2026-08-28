// Copyright 2026 cloudygreybeard
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package kusto

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// sampleResult returns a small result exercising the value types the
// renderers must handle.
func sampleResult() *Result {
	return &Result{
		Columns: []Column{
			{Name: "Name", Type: "string"},
			{Name: "Count", Type: "long"},
			{Name: "Missing", Type: "string"},
		},
		Rows: [][]any{
			{"alpha", json.Number("1"), nil},
			{"beta", json.Number("9007199254740993"), "present"},
		},
	}
}

func TestParseFormat(t *testing.T) {
	tests := []struct {
		input   string
		want    Format
		wantErr bool
	}{
		{input: "table", want: FormatTable},
		{input: "TSV", want: FormatTSV},
		{input: " csv ", want: FormatCSV},
		{input: "json", want: FormatJSON},
		{input: "ndjson", want: FormatNDJSON},
		{input: "yaml", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseFormat(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseFormat(%q) = %q, want error", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseFormat(%q) returned unexpected error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("ParseFormat(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestRenderPreservesLargeIntegers guards the reason rows are decoded with
// json.Number: a float64 would silently round this value.
func TestRenderPreservesLargeIntegers(t *testing.T) {
	for _, format := range formats {
		t.Run(string(format), func(t *testing.T) {
			var buf bytes.Buffer
			if err := Render(&buf, sampleResult(), format, RenderOptions{Headers: true}); err != nil {
				t.Fatalf("Render(%q) returned unexpected error: %v", format, err)
			}
			if !strings.Contains(buf.String(), "9007199254740993") {
				t.Errorf("Render(%q) = %q, want it to contain the exact integer", format, buf.String())
			}
		})
	}
}

func TestRenderTSV(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, sampleResult(), FormatTSV, RenderOptions{Headers: true}); err != nil {
		t.Fatalf("Render() returned unexpected error: %v", err)
	}

	want := "Name\tCount\tMissing\n" +
		"alpha\t1\t\n" +
		"beta\t9007199254740993\tpresent\n"
	if got := buf.String(); got != want {
		t.Errorf("Render(tsv) = %q, want %q", got, want)
	}
}

func TestRenderTSVWithoutHeaders(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, sampleResult(), FormatTSV, RenderOptions{}); err != nil {
		t.Fatalf("Render() returned unexpected error: %v", err)
	}
	if strings.Contains(buf.String(), "Name") {
		t.Errorf("Render(tsv) = %q, want no header row", buf.String())
	}
}

// TestRenderTSVEscapesControlCharacters checks that an embedded tab or newline
// cannot forge an extra column or row.
func TestRenderTSVEscapesControlCharacters(t *testing.T) {
	r := &Result{
		Columns: []Column{{Name: "Message", Type: "string"}},
		Rows:    [][]any{{"one\ttwo\nthree"}},
	}

	var buf bytes.Buffer
	if err := Render(&buf, r, FormatTSV, RenderOptions{}); err != nil {
		t.Fatalf("Render() returned unexpected error: %v", err)
	}

	want := `one\ttwo\nthree` + "\n"
	if got := buf.String(); got != want {
		t.Errorf("Render(tsv) = %q, want %q", got, want)
	}
}

// TestRenderCSVQuotesRatherThanEscapes checks that comma-separated output
// relies on standard quoting, so values survive a round trip.
func TestRenderCSVQuotesRatherThanEscapes(t *testing.T) {
	r := &Result{
		Columns: []Column{{Name: "Message", Type: "string"}},
		Rows:    [][]any{{`a,b "quoted"`}},
	}

	var buf bytes.Buffer
	if err := Render(&buf, r, FormatCSV, RenderOptions{}); err != nil {
		t.Fatalf("Render() returned unexpected error: %v", err)
	}

	want := `"a,b ""quoted"""` + "\n"
	if got := buf.String(); got != want {
		t.Errorf("Render(csv) = %q, want %q", got, want)
	}
}

func TestRenderJSONKeepsColumnOrder(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, sampleResult(), FormatJSON, RenderOptions{}); err != nil {
		t.Fatalf("Render() returned unexpected error: %v", err)
	}

	got := buf.String()
	name := strings.Index(got, `"Name"`)
	count := strings.Index(got, `"Count"`)
	missing := strings.Index(got, `"Missing"`)
	if name >= count || count >= missing {
		t.Errorf("Render(json) = %q, want columns in declaration order", got)
	}

	var rows []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rows); err != nil {
		t.Fatalf("Render(json) produced invalid JSON: %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("Render(json) produced %d rows, want 2", len(rows))
	}
}

func TestRenderJSONEmptyResult(t *testing.T) {
	r := &Result{Columns: []Column{{Name: "Name", Type: "string"}}}

	var buf bytes.Buffer
	if err := Render(&buf, r, FormatJSON, RenderOptions{}); err != nil {
		t.Fatalf("Render() returned unexpected error: %v", err)
	}

	var rows []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rows); err != nil {
		t.Fatalf("Render(json) produced invalid JSON: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("Render(json) produced %d rows, want 0", len(rows))
	}
}

// TestRenderJSONLeavesHTMLUnescaped checks that ampersands and angle brackets
// common in log messages survive readably.
func TestRenderJSONLeavesHTMLUnescaped(t *testing.T) {
	r := &Result{
		Columns: []Column{{Name: "URL", Type: "string"}},
		Rows:    [][]any{{"https://example.net/?a=1&b=2"}},
	}

	var buf bytes.Buffer
	if err := Render(&buf, r, FormatJSON, RenderOptions{}); err != nil {
		t.Fatalf("Render() returned unexpected error: %v", err)
	}
	if strings.Contains(buf.String(), `\u0026`) {
		t.Errorf("Render(json) = %q, want an unescaped ampersand", buf.String())
	}
}

func TestRenderNDJSONOneObjectPerLine(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, sampleResult(), FormatNDJSON, RenderOptions{}); err != nil {
		t.Fatalf("Render() returned unexpected error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("Render(ndjson) produced %d lines, want 2", len(lines))
	}
	for i, line := range lines {
		var row map[string]any
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			t.Errorf("line %d is not valid JSON: %v", i, err)
		}
	}
}

func TestRenderTableAligns(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, sampleResult(), FormatTable, RenderOptions{Headers: true}); err != nil {
		t.Fatalf("Render() returned unexpected error: %v", err)
	}

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("Render(table) produced %d lines, want 4", len(lines))
	}
	if !strings.HasPrefix(lines[1], "----") {
		t.Errorf("Render(table) second line = %q, want a rule", lines[1])
	}
	// "alpha" is padded to the width of the longer "beta" row entry.
	if !strings.HasPrefix(lines[2], "alpha") {
		t.Errorf("Render(table) third line = %q, want it to start with the first value", lines[2])
	}
	for i, line := range lines {
		if strings.HasSuffix(line, " ") {
			t.Errorf("line %d = %q, want no trailing whitespace", i, line)
		}
	}
}

func TestRenderShortRow(t *testing.T) {
	r := &Result{
		Columns: []Column{{Name: "A"}, {Name: "B"}},
		Rows:    [][]any{{"only"}},
	}

	var buf bytes.Buffer
	if err := Render(&buf, r, FormatTSV, RenderOptions{}); err != nil {
		t.Fatalf("Render() returned unexpected error: %v", err)
	}
	if want := "only\t\n"; buf.String() != want {
		t.Errorf("Render(tsv) = %q, want %q", buf.String(), want)
	}
}

func TestRenderUnknownFormat(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, sampleResult(), Format("xml"), RenderOptions{}); err == nil {
		t.Fatal("Render() succeeded, want error")
	}
}

func TestRenderValue(t *testing.T) {
	tests := []struct {
		name  string
		input any
		want  string
	}{
		{name: "nil", input: nil, want: ""},
		{name: "string", input: "text", want: "text"},
		{name: "number", input: json.Number("42"), want: "42"},
		{name: "true", input: true, want: "true"},
		{name: "false", input: false, want: "false"},
		{name: "dynamic", input: map[string]any{"k": "v"}, want: `{"k":"v"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := renderValue(tt.input); got != tt.want {
				t.Errorf("renderValue(%v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
