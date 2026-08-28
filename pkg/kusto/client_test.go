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
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cloudygreybeard/kql/pkg/auth"
)

// fixedProvider supplies a constant token.
type fixedProvider struct {
	token string
	err   error
}

func (f fixedProvider) Name() string { return "fixed" }

func (f fixedProvider) Token(context.Context, string) (auth.Token, error) {
	if f.err != nil {
		return auth.Token{}, f.err
	}
	return auth.Token{Value: f.token, ExpiresAt: time.Now().Add(time.Hour)}, nil
}

// primaryResultResponse is a minimal version 2 response carrying one row.
const primaryResultResponse = `[
  {"FrameType":"DataSetHeader","IsProgressive":false,"Version":"v2.0"},
  {"FrameType":"DataTable","TableId":0,"TableKind":"QueryProperties",
   "Columns":[{"ColumnName":"Value","ColumnType":"string"}],"Rows":[["ignored"]]},
  {"FrameType":"DataTable","TableId":1,"TableKind":"PrimaryResult","TableName":"PrimaryResult",
   "Columns":[{"ColumnName":"Name","ColumnType":"string"},{"ColumnName":"Count","ColumnType":"long"}],
   "Rows":[["alpha",9007199254740993]]},
  {"FrameType":"DataTable","TableId":2,"TableKind":"QueryCompletionInformation",
   "Columns":[{"ColumnName":"EventTypeName","ColumnType":"string"},{"ColumnName":"Payload","ColumnType":"string"}],
   "Rows":[["QueryResourceConsumption","{\"ExecutionTime\":0.015}"]]},
  {"FrameType":"DataSetCompletion","HasErrors":false,"Cancelled":false}
]`

// newTestClient returns a client pointed at a server running handler.
func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	return &Client{
		Endpoint: server.URL,
		Provider: fixedProvider{token: "test-token"},
		HTTP:     server.Client(),
	}
}

func TestClientQuery(t *testing.T) {
	var gotBody requestBody
	var gotAuth, gotContentType string

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotContentType = r.Header.Get("Content-Type")
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, primaryResultResponse)
	})

	got, err := client.Query(context.Background(), QueryRequest{
		Database:      "Samples",
		Query:         "StormEvents | take 1",
		ServerTimeout: 90 * time.Second,
	})
	if err != nil {
		t.Fatalf("Query() returned unexpected error: %v", err)
	}

	if want := "Bearer test-token"; gotAuth != want {
		t.Errorf("Authorization header = %q, want %q", gotAuth, want)
	}
	if !strings.HasPrefix(gotContentType, "application/json") {
		t.Errorf("Content-Type header = %q, want application/json", gotContentType)
	}
	if gotBody.DB != "Samples" {
		t.Errorf("request db = %q, want %q", gotBody.DB, "Samples")
	}
	if gotBody.CSL != "StormEvents | take 1" {
		t.Errorf("request csl = %q, want %q", gotBody.CSL, "StormEvents | take 1")
	}
	if gotBody.Properties == nil {
		t.Fatal("request carried no properties, want a server timeout")
	}
	if want := "00:01:30"; gotBody.Properties.Options["servertimeout"] != want {
		t.Errorf("servertimeout = %v, want %q", gotBody.Properties.Options["servertimeout"], want)
	}

	if len(got.Columns) != 2 {
		t.Fatalf("Query() returned %d columns, want 2", len(got.Columns))
	}
	if len(got.Rows) != 1 {
		t.Fatalf("Query() returned %d rows, want 1", len(got.Rows))
	}
	// The primary result must be selected, not the QueryProperties table.
	if got.Columns[0].Name != "Name" {
		t.Errorf("first column = %q, want %q", got.Columns[0].Name, "Name")
	}
	if value := renderValue(got.Rows[0][1]); value != "9007199254740993" {
		t.Errorf("Query() row value = %q, want the exact integer", value)
	}
	if len(got.Stats) != 1 || got.Stats[0].Name != "QueryResourceConsumption" {
		t.Errorf("Query() stats = %+v, want one QueryResourceConsumption entry", got.Stats)
	}
}

func TestClientQueryOmitsPropertiesWhenUnset(t *testing.T) {
	var raw map[string]any

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &raw)
		_, _ = io.WriteString(w, primaryResultResponse)
	})

	if _, err := client.Query(context.Background(), QueryRequest{
		Database: "Samples",
		Query:    "StormEvents | count",
	}); err != nil {
		t.Fatalf("Query() returned unexpected error: %v", err)
	}

	if _, ok := raw["properties"]; ok {
		t.Errorf("request carried properties = %v, want them omitted", raw["properties"])
	}
}

func TestClientQueryServiceError(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":{"code":"BadRequest_SyntaxError",
			"message":"Request is invalid","@message":"Syntax error near 'seelct'"}}`)
	})

	_, err := client.Query(context.Background(), QueryRequest{Database: "Samples", Query: "seelct"})
	if err == nil {
		t.Fatal("Query() succeeded, want error")
	}
	for _, want := range []string{"BadRequest_SyntaxError", "Syntax error near 'seelct'"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Query() error = %q, want it to contain %q", err, want)
		}
	}
}

func TestClientQueryCompletionErrors(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `[
		  {"FrameType":"DataTable","TableId":1,"TableKind":"PrimaryResult",
		   "Columns":[{"ColumnName":"A","ColumnType":"string"}],"Rows":[]},
		  {"FrameType":"DataSetCompletion","HasErrors":true}
		]`)
	})

	if _, err := client.Query(context.Background(), QueryRequest{
		Database: "Samples",
		Query:    "T | take 1",
	}); err == nil {
		t.Fatal("Query() succeeded, want error")
	}
}

func TestClientQueryNoPrimaryResult(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `[{"FrameType":"DataSetHeader","Version":"v2.0"}]`)
	})

	if _, err := client.Query(context.Background(), QueryRequest{
		Database: "Samples",
		Query:    "T | take 1",
	}); err == nil {
		t.Fatal("Query() succeeded, want error")
	}
}

func TestClientQueryTokenFailure(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("request was sent despite a token failure")
	})
	client.Provider = fixedProvider{err: io.ErrUnexpectedEOF}

	if _, err := client.Query(context.Background(), QueryRequest{
		Database: "Samples",
		Query:    "T | take 1",
	}); err == nil {
		t.Fatal("Query() succeeded, want error")
	}
}

func TestClientQueryValidation(t *testing.T) {
	tests := []struct {
		name string
		req  QueryRequest
	}{
		{name: "no database", req: QueryRequest{Query: "T"}},
		{name: "no query", req: QueryRequest{Database: "Samples"}},
		{name: "blank query", req: QueryRequest{Database: "Samples", Query: "   "}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &Client{Endpoint: "https://example.net", Provider: fixedProvider{token: "t"}}
			if _, err := client.Query(context.Background(), tt.req); err == nil {
				t.Fatal("Query() succeeded, want error")
			}
		})
	}
}

func TestClientQueryNoEndpoint(t *testing.T) {
	client := &Client{Provider: fixedProvider{token: "t"}}
	if _, err := client.Query(context.Background(), QueryRequest{
		Database: "Samples",
		Query:    "T",
	}); err == nil {
		t.Fatal("Query() succeeded, want error")
	}
}

func TestTimespan(t *testing.T) {
	tests := []struct {
		input time.Duration
		want  string
	}{
		{input: 30 * time.Second, want: "00:00:30"},
		{input: 4 * time.Minute, want: "00:04:00"},
		{input: 90 * time.Minute, want: "01:30:00"},
		{input: 1500 * time.Millisecond, want: "00:00:02"},
	}

	for _, tt := range tests {
		t.Run(tt.input.String(), func(t *testing.T) {
			if got := timespan(tt.input); got != tt.want {
				t.Errorf("timespan(%v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestBuildPropertiesOmitsUnrequestedServerTimeout(t *testing.T) {
	// Sending a timeout kql invented would override the cluster's own default
	// and any workload group policy, so none is sent unless one is asked for.
	if got := buildProperties(QueryRequest{Database: "Samples", Query: "T"}); got != nil {
		t.Fatalf("buildProperties() = %+v, want nil", got)
	}

	got := buildProperties(QueryRequest{ServerTimeout: 90 * time.Second})
	if got == nil {
		t.Fatal("buildProperties() = nil, want a server timeout")
	}
	if want := "00:01:30"; got.Options["servertimeout"] != want {
		t.Errorf("servertimeout = %v, want %q", got.Options["servertimeout"], want)
	}
}
