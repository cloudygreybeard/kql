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
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/cloudygreybeard/kql/pkg/auth"
)

// queryPath is the data-plane query endpoint.
//
// Version 2 is used in preference to version 1 because it reports column types
// alongside the data, which the renderers rely on.
const queryPath = "/v2/rest/query"

// maxErrorBody bounds how much of an unexpected response body is quoted back
// to the user.
const maxErrorBody = 4096

// Column describes one column of a result table.
type Column struct {
	// Name is the column's name.
	Name string `json:"ColumnName"`
	// Type is the Kusto scalar type, such as "string" or "datetime".
	Type string `json:"ColumnType"`
}

// Result is the primary table produced by a query.
type Result struct {
	// Columns describes the shape of each row.
	Columns []Column
	// Rows holds the returned values, positionally matching Columns. Numbers
	// are held as [json.Number] so that 64-bit values survive intact.
	Rows [][]any
	// Stats holds the query completion information reported by the cluster,
	// keyed by event name.
	Stats []Stat
}

// Stat is one entry of a query's completion information.
type Stat struct {
	// Name identifies the reported event, such as "QueryResourceConsumption".
	Name string
	// Payload is the reported value, rendered as text.
	Payload string
}

// Client executes queries against a single Kusto cluster.
type Client struct {
	// Endpoint is the cluster base URL, such as
	// "https://help.kusto.windows.net".
	Endpoint string
	// Provider supplies bearer tokens for Endpoint.
	Provider auth.Provider
	// HTTP performs the request. A nil value uses [http.DefaultClient].
	HTTP *http.Client
	// Resource overrides the token audience. An empty value uses the
	// audience derived from Endpoint.
	Resource string
}

// QueryRequest describes a single query execution.
type QueryRequest struct {
	// Database is the database to run against.
	Database string
	// Query is the KQL text.
	Query string
	// Parameters supplies values for parameters the query declares with
	// "declare query_parameters".
	Parameters map[string]string
	// ServerTimeout bounds execution at the cluster. Zero uses the cluster
	// default.
	ServerTimeout time.Duration
	// NoTruncation disables the cluster's result size limits.
	NoTruncation bool
}

// requestBody is the JSON body of a query request.
type requestBody struct {
	DB         string      `json:"db"`
	CSL        string      `json:"csl"`
	Properties *properties `json:"properties,omitempty"`
}

// properties carries client request properties.
type properties struct {
	Options    map[string]any    `json:"Options,omitempty"`
	Parameters map[string]string `json:"Parameters,omitempty"`
}

// frame is one element of a version 2 response.
type frame struct {
	FrameType string          `json:"FrameType"`
	TableKind string          `json:"TableKind"`
	Columns   []Column        `json:"Columns"`
	Rows      json.RawMessage `json:"Rows"`
	HasErrors bool            `json:"HasErrors"`
}

// serviceError is the body returned when the cluster rejects a request.
type serviceError struct {
	Error struct {
		Code       string `json:"code"`
		Message    string `json:"message"`
		AtMessage  string `json:"@message"`
		InnerError *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"@innererror"`
	} `json:"error"`
}

// message renders the most specific description the cluster supplied.
func (e serviceError) message() string {
	parts := make([]string, 0, 3)
	if e.Error.Code != "" {
		parts = append(parts, e.Error.Code)
	}
	for _, s := range []string{e.Error.AtMessage, e.Error.Message} {
		if s != "" {
			parts = append(parts, s)
			break
		}
	}
	if e.Error.InnerError != nil && e.Error.InnerError.Message != "" {
		parts = append(parts, e.Error.InnerError.Message)
	}
	return strings.Join(parts, ": ")
}

// Query executes req and returns its primary result table.
func (c *Client) Query(ctx context.Context, req QueryRequest) (*Result, error) {
	if c.Endpoint == "" {
		return nil, fmt.Errorf("client endpoint must not be empty")
	}
	if req.Database == "" {
		return nil, fmt.Errorf("database must not be empty")
	}
	if strings.TrimSpace(req.Query) == "" {
		return nil, fmt.Errorf("query must not be empty")
	}

	httpReq, err := c.newRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	client := c.HTTP
	if client == nil {
		client = http.DefaultClient
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("querying %s: %w", c.Endpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response from %s: %w", c.Endpoint, err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, responseError(resp.StatusCode, body)
	}
	return parseFrames(body)
}

// newRequest builds the HTTP request for req, acquiring a bearer token.
func (c *Client) newRequest(ctx context.Context, req QueryRequest) (*http.Request, error) {
	payload, err := json.Marshal(requestBody{
		DB:         req.Database,
		CSL:        req.Query,
		Properties: buildProperties(req),
	})
	if err != nil {
		return nil, fmt.Errorf("encoding request: %w", err)
	}

	resource := c.Resource
	if resource == "" {
		resource = Resource(c.Endpoint)
	}

	token, err := c.Provider.Token(ctx, resource)
	if err != nil {
		return nil, fmt.Errorf("acquiring token: %w", err)
	}

	url := strings.TrimSuffix(c.Endpoint, "/") + queryPath
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+token.Value)
	httpReq.Header.Set("Content-Type", "application/json; charset=utf-8")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("x-ms-client-request-id", "KQL.Query;"+requestID())
	httpReq.Header.Set("x-ms-app", "kql")
	return httpReq, nil
}

// buildProperties assembles client request properties, returning nil when the
// request carries none.
func buildProperties(req QueryRequest) *properties {
	options := make(map[string]any)
	if req.ServerTimeout > 0 {
		options["servertimeout"] = timespan(req.ServerTimeout)
	}
	if req.NoTruncation {
		options["notruncation"] = true
	}
	if len(options) == 0 && len(req.Parameters) == 0 {
		return nil
	}
	return &properties{Options: options, Parameters: req.Parameters}
}

// timespan renders d in the hh:mm:ss form Kusto expects.
func timespan(d time.Duration) string {
	d = d.Round(time.Second)
	h := int(d / time.Hour)
	m := int((d % time.Hour) / time.Minute)
	s := int((d % time.Minute) / time.Second)
	return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
}

// requestID returns a random identifier correlating a request with cluster
// diagnostics.
func requestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(b[:])
}

// responseError converts a non-success response into an error.
func responseError(status int, body []byte) error {
	var svc serviceError
	if err := json.Unmarshal(body, &svc); err == nil {
		if msg := svc.message(); msg != "" {
			return fmt.Errorf("kusto returned %s: %s", http.StatusText(status), msg)
		}
	}
	return fmt.Errorf("kusto returned %d %s: %s",
		status, http.StatusText(status), clip(body, maxErrorBody))
}

// clip shortens b for inclusion in an error message.
func clip(b []byte, n int) string {
	s := strings.TrimSpace(string(b))
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// parseFrames extracts the primary result and completion information from a
// version 2 response.
func parseFrames(body []byte) (*Result, error) {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()

	var frames []frame
	if err := dec.Decode(&frames); err != nil {
		// A rejected query may answer with an error object rather than the
		// expected array of frames.
		var svc serviceError
		if jsonErr := json.Unmarshal(body, &svc); jsonErr == nil && svc.message() != "" {
			return nil, fmt.Errorf("kusto rejected the query: %s", svc.message())
		}
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	result := &Result{}
	found := false

	for _, f := range frames {
		switch {
		case f.FrameType == "DataTable" && f.TableKind == "PrimaryResult":
			rows, err := decodeRows(f.Rows)
			if err != nil {
				return nil, err
			}
			result.Columns = f.Columns
			result.Rows = rows
			found = true

		case f.FrameType == "DataTable" && f.TableKind == "QueryCompletionInformation":
			result.Stats = decodeStats(f)

		case f.FrameType == "DataSetCompletion" && f.HasErrors:
			return nil, fmt.Errorf("query completed with errors reported by the cluster")
		}
	}

	if !found {
		return nil, fmt.Errorf("response contained no primary result")
	}
	return result, nil
}

// decodeRows converts a frame's rows, preserving numeric fidelity.
func decodeRows(raw json.RawMessage) ([][]any, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()

	var rows [][]any
	if err := dec.Decode(&rows); err != nil {
		return nil, fmt.Errorf("parsing result rows: %w", err)
	}
	return rows, nil
}

// decodeStats extracts completion information, which the cluster reports as a
// table whose columns include an event name and its payload.
func decodeStats(f frame) []Stat {
	rows, err := decodeRows(f.Rows)
	if err != nil {
		return nil
	}

	name, payload := -1, -1
	for i, col := range f.Columns {
		switch col.Name {
		case "EventTypeName":
			name = i
		case "Payload":
			payload = i
		}
	}
	if name < 0 || payload < 0 {
		return nil
	}

	stats := make([]Stat, 0, len(rows))
	for _, row := range rows {
		if name >= len(row) || payload >= len(row) {
			continue
		}
		stats = append(stats, Stat{
			Name:    renderValue(row[name]),
			Payload: renderValue(row[payload]),
		})
	}
	return stats
}
