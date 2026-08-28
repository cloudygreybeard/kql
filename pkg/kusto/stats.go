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
	"fmt"
	"strconv"
	"strings"
)

// resourceConsumption is the event name under which the cluster reports the
// cost of executing a query.
const resourceConsumption = "QueryResourceConsumption"

// Metric is a single named measurement drawn from the query statistics.
type Metric struct {
	// Name is the human-readable label, such as "cpu".
	Name string
	// Value is the formatted measurement, such as "00:00:00.015".
	Value string
}

// digestField describes one measurement to lift out of the resource
// consumption payload, which is a nested JSON object.
type digestField struct {
	// label names the metric in the digest.
	label string
	// path is the sequence of object keys leading to the value.
	path []string
	// bytes marks values that should be scaled to human-readable units.
	bytes bool
}

// digestFields lists the measurements worth reporting by default, in the order
// they are printed. The full payload runs to several thousand characters, most
// of it cache and shard detail that matters only when tuning a specific query,
// so the digest keeps to the figures that explain how long a query took and how
// much data it touched.
var digestFields = []digestField{
	{label: "execution time", path: []string{"ExecutionTime"}},
	{label: "cpu", path: []string{"resource_usage", "cpu", "total cpu"}},
	{label: "memory peak per node", path: []string{"resource_usage", "memory", "peak_per_node"}, bytes: true},
	{label: "rows scanned", path: []string{"input_dataset_statistics", "rows", "scanned"}},
	{label: "rows total", path: []string{"input_dataset_statistics", "rows", "total"}},
	{label: "extents scanned", path: []string{"input_dataset_statistics", "extents", "scanned"}},
	{label: "extents total", path: []string{"input_dataset_statistics", "extents", "total"}},
	{label: "cross-cluster bytes", path: []string{"resource_usage", "network", "cross_cluster_total_bytes"}, bytes: true},
}

// Summarize reduces the query statistics to the handful of measurements that
// describe a query's cost. It returns nil when no resource consumption was
// reported, or when the payload cannot be parsed, so that callers can fall back
// to the raw statistics rather than lose them.
func Summarize(stats []Stat) []Metric {
	payload := ""
	for _, stat := range stats {
		if stat.Name == resourceConsumption {
			payload = stat.Payload
			break
		}
	}
	if payload == "" {
		return nil
	}

	var root map[string]any
	dec := json.NewDecoder(strings.NewReader(payload))
	dec.UseNumber()
	if err := dec.Decode(&root); err != nil {
		return nil
	}

	metrics := make([]Metric, 0, len(digestFields))
	for _, field := range digestFields {
		value, ok := lookup(root, field.path)
		if !ok {
			continue
		}
		text := renderValue(value)
		if field.bytes {
			text = humanizeBytes(value)
		}
		metrics = append(metrics, Metric{Name: field.label, Value: text})
	}
	if len(metrics) == 0 {
		return nil
	}
	return metrics
}

// lookup walks a decoded JSON object along path, reporting whether the full
// path resolved to a value.
func lookup(root map[string]any, path []string) (any, bool) {
	var current any = root
	for _, key := range path {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = object[key]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

// byteUnits are the successive scales applied by humanizeBytes.
var byteUnits = []string{"B", "KiB", "MiB", "GiB", "TiB", "PiB"}

// humanizeBytes renders a byte count in binary units, leaving values it cannot
// interpret as numbers untouched.
func humanizeBytes(value any) string {
	text := renderValue(value)
	n, err := strconv.ParseFloat(text, 64)
	if err != nil || n < 0 {
		return text
	}

	scaled, unit := n, byteUnits[0]
	for _, next := range byteUnits[1:] {
		if scaled < 1024 {
			break
		}
		scaled /= 1024
		unit = next
	}
	if unit == byteUnits[0] {
		return fmt.Sprintf("%s B", text)
	}
	return fmt.Sprintf("%.1f %s", scaled, unit)
}
