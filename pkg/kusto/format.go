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
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

// Format identifies an output rendering.
type Format string

// Supported output formats.
const (
	// FormatTable aligns columns for reading at a terminal.
	FormatTable Format = "table"
	// FormatTSV emits tab-separated values.
	FormatTSV Format = "tsv"
	// FormatCSV emits comma-separated values with standard quoting.
	FormatCSV Format = "csv"
	// FormatJSON emits one JSON array of objects.
	FormatJSON Format = "json"
	// FormatNDJSON emits one JSON object per line.
	FormatNDJSON Format = "ndjson"
)

// formats lists the supported formats in the order they are documented.
var formats = []Format{FormatTable, FormatTSV, FormatCSV, FormatJSON, FormatNDJSON}

// ParseFormat converts a flag value into a Format.
func ParseFormat(s string) (Format, error) {
	candidate := Format(strings.ToLower(strings.TrimSpace(s)))
	for _, f := range formats {
		if candidate == f {
			return f, nil
		}
	}
	names := make([]string, len(formats))
	for i, f := range formats {
		names[i] = string(f)
	}
	return "", fmt.Errorf("unknown format %q (want %s)", s, strings.Join(names, ", "))
}

// RenderOptions adjusts how a result is written.
type RenderOptions struct {
	// Headers writes a header row for the tabular formats.
	Headers bool
}

// Render writes r to w in the given format.
func Render(w io.Writer, r *Result, format Format, opts RenderOptions) error {
	switch format {
	case FormatTable:
		return renderTable(w, r, opts)
	case FormatTSV:
		return renderSeparated(w, r, opts, '\t')
	case FormatCSV:
		return renderCSV(w, r, opts)
	case FormatJSON:
		return renderJSON(w, r)
	case FormatNDJSON:
		return renderNDJSON(w, r)
	default:
		return fmt.Errorf("unknown format %q", format)
	}
}

// renderValue converts a cell into its text representation.
//
// Numbers arrive as [json.Number] and are emitted verbatim, so that 64-bit
// integers and high-precision decimals are not degraded by conversion through
// a float. Dynamic columns are re-encoded as compact JSON.
func renderValue(v any) string {
	switch value := v.(type) {
	case nil:
		return ""
	case string:
		return value
	case json.Number:
		return value.String()
	case bool:
		if value {
			return "true"
		}
		return "false"
	default:
		encoded, err := marshalValue(value)
		if err != nil {
			return fmt.Sprint(value)
		}
		return string(encoded)
	}
}

// marshalValue encodes v as JSON without escaping HTML metacharacters, which
// would otherwise obscure the ampersands and angle brackets common in log
// messages and URLs.
func marshalValue(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// escapeControl replaces the characters that would otherwise disturb a
// line-oriented rendering.
func escapeControl(s string) string {
	replacer := strings.NewReplacer("\t", `\t`, "\n", `\n`, "\r", `\r`)
	return replacer.Replace(s)
}

// cell returns the value at index i of row, or the empty string when the row
// is short.
func cell(row []any, i int) any {
	if i < len(row) {
		return row[i]
	}
	return nil
}

// renderTable writes aligned columns.
func renderTable(w io.Writer, r *Result, opts RenderOptions) error {
	if len(r.Columns) == 0 {
		return nil
	}

	widths := make([]int, len(r.Columns))
	if opts.Headers {
		for i, col := range r.Columns {
			widths[i] = utf8.RuneCountInString(col.Name)
		}
	}

	text := make([][]string, len(r.Rows))
	for i, row := range r.Rows {
		text[i] = make([]string, len(r.Columns))
		for j := range r.Columns {
			value := escapeControl(renderValue(cell(row, j)))
			text[i][j] = value
			if n := utf8.RuneCountInString(value); n > widths[j] {
				widths[j] = n
			}
		}
	}

	bw := newWriter(w)
	if opts.Headers {
		names := make([]string, len(r.Columns))
		rules := make([]string, len(r.Columns))
		for i, col := range r.Columns {
			names[i] = col.Name
			rules[i] = strings.Repeat("-", widths[i])
		}
		bw.writeRow(names, widths)
		bw.writeRow(rules, widths)
	}
	for _, row := range text {
		bw.writeRow(row, widths)
	}
	return bw.err
}

// tableWriter accumulates the first write error encountered.
type tableWriter struct {
	w   io.Writer
	err error
}

// newWriter returns a tableWriter over w.
func newWriter(w io.Writer) *tableWriter {
	return &tableWriter{w: w}
}

// writeRow writes one padded row. Trailing padding is removed so that lines
// carry no trailing whitespace, including when the final value is empty.
func (t *tableWriter) writeRow(values []string, widths []int) {
	if t.err != nil {
		return
	}
	var line strings.Builder
	for i, value := range values {
		if i > 0 {
			line.WriteString("  ")
		}
		line.WriteString(value)
		if i < len(values)-1 {
			if pad := widths[i] - utf8.RuneCountInString(value); pad > 0 {
				line.WriteString(strings.Repeat(" ", pad))
			}
		}
	}
	_, t.err = io.WriteString(t.w, strings.TrimRight(line.String(), " ")+"\n")
}

// renderSeparated writes values joined by sep, escaping characters that would
// break the line-oriented structure.
func renderSeparated(w io.Writer, r *Result, opts RenderOptions, sep rune) error {
	var buf bytes.Buffer
	if opts.Headers {
		names := make([]string, len(r.Columns))
		for i, col := range r.Columns {
			names[i] = escapeControl(col.Name)
		}
		buf.WriteString(strings.Join(names, string(sep)))
		buf.WriteByte('\n')
	}

	values := make([]string, len(r.Columns))
	for _, row := range r.Rows {
		for i := range r.Columns {
			values[i] = escapeControl(renderValue(cell(row, i)))
		}
		buf.WriteString(strings.Join(values, string(sep)))
		buf.WriteByte('\n')
	}

	_, err := w.Write(buf.Bytes())
	return err
}

// renderCSV writes comma-separated values, relying on standard quoting rather
// than escaping so that embedded separators survive intact.
func renderCSV(w io.Writer, r *Result, opts RenderOptions) error {
	cw := csv.NewWriter(w)
	if opts.Headers {
		names := make([]string, len(r.Columns))
		for i, col := range r.Columns {
			names[i] = col.Name
		}
		if err := cw.Write(names); err != nil {
			return err
		}
	}

	values := make([]string, len(r.Columns))
	for _, row := range r.Rows {
		for i := range r.Columns {
			values[i] = renderValue(cell(row, i))
		}
		if err := cw.Write(values); err != nil {
			return err
		}
	}

	cw.Flush()
	return cw.Error()
}

// renderJSON writes the result as an array of objects.
func renderJSON(w io.Writer, r *Result) error {
	var buf bytes.Buffer
	buf.WriteString("[\n")
	for i, row := range r.Rows {
		if i > 0 {
			buf.WriteString(",\n")
		}
		buf.WriteString("  ")
		if err := writeObject(&buf, r.Columns, row); err != nil {
			return err
		}
	}
	if len(r.Rows) > 0 {
		buf.WriteByte('\n')
	}
	buf.WriteString("]\n")

	_, err := w.Write(buf.Bytes())
	return err
}

// renderNDJSON writes one object per line.
func renderNDJSON(w io.Writer, r *Result) error {
	var buf bytes.Buffer
	for _, row := range r.Rows {
		if err := writeObject(&buf, r.Columns, row); err != nil {
			return err
		}
		buf.WriteByte('\n')
	}
	_, err := w.Write(buf.Bytes())
	return err
}

// writeObject writes one row as a JSON object.
//
// The object is assembled by hand rather than through a map so that columns
// keep the order the query produced them in.
func writeObject(buf *bytes.Buffer, cols []Column, row []any) error {
	buf.WriteByte('{')
	for i, col := range cols {
		if i > 0 {
			buf.WriteByte(',')
		}
		key, err := marshalValue(col.Name)
		if err != nil {
			return fmt.Errorf("encoding column %q: %w", col.Name, err)
		}
		buf.Write(key)
		buf.WriteByte(':')

		value, err := marshalValue(cell(row, i))
		if err != nil {
			return fmt.Errorf("encoding value for column %q: %w", col.Name, err)
		}
		buf.Write(value)
	}
	buf.WriteByte('}')
	return nil
}
