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
	"encoding/json"
	"testing"
)

// realPayload mirrors the shape the service returns, trimmed to the fields the
// digest reads plus surrounding detail it must ignore.
const realPayload = `{
  "ExecutionTime": 0.0156248,
  "resource_usage": {
    "cache": {"shards": {"hot": {"hitbytes": 1024, "missbytes": 0}}},
    "cpu": {"user": "00:00:00", "kernel": "00:00:00.0156250", "total cpu": "00:00:00.0156250"},
    "memory": {"peak_per_node": 5343232},
    "network": {"inter_cluster_total_bytes": 3427, "cross_cluster_total_bytes": 0}
  },
  "input_dataset_statistics": {
    "extents": {"total": 12, "scanned": 3},
    "rows": {"total": 4096, "scanned": 128}
  },
  "dataset_statistics": [{"table_row_count": 1, "table_size": 9}]
}`

func TestSummarize(t *testing.T) {
	stats := []Stat{
		{Name: "QueryInfo", Payload: `{"Card":"1"}`},
		{Name: resourceConsumption, Payload: realPayload},
	}

	got := Summarize(stats)
	want := []Metric{
		{Name: "execution time", Value: "0.0156248"},
		{Name: "cpu", Value: "00:00:00.0156250"},
		{Name: "memory peak per node", Value: "5.1 MiB"},
		{Name: "rows scanned", Value: "128"},
		{Name: "rows total", Value: "4096"},
		{Name: "extents scanned", Value: "3"},
		{Name: "extents total", Value: "12"},
		{Name: "cross-cluster bytes", Value: "0 B"},
	}

	if len(got) != len(want) {
		t.Fatalf("Summarize() returned %d metrics, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Summarize()[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestSummarizeReturnsNil(t *testing.T) {
	tests := []struct {
		name  string
		stats []Stat
	}{
		{name: "no stats", stats: nil},
		{name: "no consumption event", stats: []Stat{{Name: "QueryInfo", Payload: `{}`}}},
		{name: "unparseable payload", stats: []Stat{{Name: resourceConsumption, Payload: "not json"}}},
		{name: "no known fields", stats: []Stat{{Name: resourceConsumption, Payload: `{"other":1}`}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Summarize(tt.stats); got != nil {
				t.Errorf("Summarize() = %+v, want nil so the caller falls back to the raw payload", got)
			}
		})
	}
}

func TestHumanizeBytes(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  string
	}{
		{name: "zero", value: json.Number("0"), want: "0 B"},
		{name: "below one kibibyte", value: json.Number("512"), want: "512 B"},
		{name: "exactly one kibibyte", value: json.Number("1024"), want: "1.0 KiB"},
		{name: "mebibytes", value: json.Number("5343232"), want: "5.1 MiB"},
		{name: "gibibytes", value: json.Number("2147483648"), want: "2.0 GiB"},
		{name: "non-numeric passes through", value: "n/a", want: "n/a"},
		{name: "negative passes through", value: json.Number("-1"), want: "-1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := humanizeBytes(tt.value); got != tt.want {
				t.Errorf("humanizeBytes(%v) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

func TestLookup(t *testing.T) {
	root := map[string]any{
		"a": map[string]any{"b": map[string]any{"c": "found"}},
		"s": "scalar",
	}

	if got, ok := lookup(root, []string{"a", "b", "c"}); !ok || got != "found" {
		t.Errorf("lookup(a.b.c) = %v, %v, want \"found\", true", got, ok)
	}
	if _, ok := lookup(root, []string{"a", "missing"}); ok {
		t.Error("lookup(a.missing) reported found, want not found")
	}
	if _, ok := lookup(root, []string{"s", "b"}); ok {
		t.Error("lookup through a scalar reported found, want not found")
	}
}
