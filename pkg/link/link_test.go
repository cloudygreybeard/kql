// Copyright 2024 cloudygreybeard
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

package link

import (
	"strings"
	"testing"
)

func TestBuild(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		cluster  string
		database string
		baseURL  string
		wantErr  bool
	}{
		{
			name:     "simple query",
			query:    "StormEvents | take 10",
			cluster:  "help",
			database: "Samples",
			baseURL:  "",
			wantErr:  false,
		},
		{
			name:     "complex query",
			query:    "let start_time = ago(7d);\nlet end_time = now();\nAllServiceLogs(startTime=start_time, endTime=end_time)\n| where RESOURCE_ID == \"test\"\n| take 100",
			cluster:  "mycluster.westeurope",
			database: "ARORPLogs",
			baseURL:  "",
			wantErr:  false,
		},
		{
			name:     "custom base URL",
			query:    "print 'hello'",
			cluster:  "test",
			database: "testdb",
			baseURL:  "https://custom.kusto.example.com",
			wantErr:  false,
		},
		{
			name:     "empty query",
			query:    "",
			cluster:  "test",
			database: "testdb",
			baseURL:  "",
			wantErr:  true,
		},
		{
			name:     "empty cluster",
			query:    "print 1",
			cluster:  "",
			database: "testdb",
			baseURL:  "",
			wantErr:  true,
		},
		{
			name:     "empty database",
			query:    "print 1",
			cluster:  "test",
			database: "",
			baseURL:  "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			link, err := Build(tt.query, tt.cluster, tt.database, tt.baseURL)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Build() expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Build() unexpected error: %v", err)
				return
			}

			// Verify the link structure
			expectedBase := tt.baseURL
			if expectedBase == "" {
				expectedBase = DefaultBaseURL
			}
			if !strings.HasPrefix(link, expectedBase) {
				t.Errorf("Build() link does not start with expected base URL: got %s", link)
			}

			if !strings.Contains(link, tt.cluster) {
				t.Errorf("Build() link does not contain cluster: got %s", link)
			}

			if !strings.Contains(link, tt.database) {
				t.Errorf("Build() link does not contain database: got %s", link)
			}

			if !strings.Contains(link, "query=") {
				t.Errorf("Build() link does not contain query parameter: got %s", link)
			}
		})
	}
}

func TestExtract(t *testing.T) {
	// First build a link, then extract from it
	originalQuery := "StormEvents\n| where StartTime > ago(7d)\n| summarize count() by State\n| top 10 by count_"

	link, err := Build(originalQuery, "help", "Samples", "")
	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}

	extractedQuery, err := Extract(link)
	if err != nil {
		t.Fatalf("Extract() failed: %v", err)
	}

	if extractedQuery != originalQuery {
		t.Errorf("Extract() returned different query:\ngot:  %q\nwant: %q", extractedQuery, originalQuery)
	}
}

func TestRoundTrip(t *testing.T) {
	queries := []string{
		"print 'hello world'",
		"StormEvents | take 10",
		"let x = 1;\nlet y = 2;\nprint x + y",
		`AllServiceLogs(startTime=ago(7d), endTime=now())
| where RESOURCE_ID == tolower("/subscriptions/xxx/resourcegroups/yyy/providers/microsoft.redhatopenshift/openshiftclusters/zzz")
| where REQUEST_PATH has "maintenanceManifests"
| project PreciseTimeStamp, REQUEST_METHOD, MESSAGE
| order by PreciseTimeStamp desc`,
		// Unicode and special characters
		"print '日本語テスト'",
		"print 'émojis: 🎉🚀'",
	}

	for _, query := range queries {
		t.Run("roundtrip", func(t *testing.T) {
			link, err := Build(query, "testcluster", "testdb", "")
			if err != nil {
				t.Fatalf("Build() failed: %v", err)
			}

			extracted, err := Extract(link)
			if err != nil {
				t.Fatalf("Extract() failed: %v", err)
			}

			if extracted != query {
				t.Errorf("Round trip failed:\noriginal:  %q\nextracted: %q", query, extracted)
			}
		})
	}
}

func TestExtractErrors(t *testing.T) {
	tests := []struct {
		name    string
		link    string
		wantErr bool
	}{
		{
			name:    "invalid URL",
			link:    "not a url at all %%%",
			wantErr: true,
		},
		{
			name:    "no query parameter",
			link:    "https://dataexplorer.azure.com/clusters/help/databases/Samples",
			wantErr: true,
		},
		{
			name:    "invalid base64",
			link:    "https://dataexplorer.azure.com/clusters/help/databases/Samples?query=!!!invalid!!!",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Extract(tt.link)
			if tt.wantErr && err == nil {
				t.Errorf("Extract() expected error but got none")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("Extract() unexpected error: %v", err)
			}
		})
	}
}

func TestBuildWithTrailingSlashBaseURL(t *testing.T) {
	// Ensure trailing slash in base URL is handled
	link, err := Build("print 1", "test", "testdb", "https://example.com/")
	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}

	// Should not have double slash
	if strings.Contains(link, "//clusters") {
		t.Errorf("Build() produced double slash: %s", link)
	}
}

func TestExtractInvalidGzip(t *testing.T) {
	// Valid base64 but invalid gzip data
	// "not gzip data" in base64 is "bm90IGd6aXAgZGF0YQ=="
	link := "https://dataexplorer.azure.com/clusters/help/databases/Samples?query=bm90IGd6aXAgZGF0YQ=="
	_, err := Extract(link)
	if err == nil {
		t.Error("Extract() expected error for invalid gzip data")
	}
}

func TestExtractVeryLongQuery(t *testing.T) {
	// Test with a very long query to ensure compression works
	longQuery := strings.Repeat("StormEvents | where State == 'TEXAS' | ", 100)

	link, err := Build(longQuery, "help", "Samples", "")
	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}

	extracted, err := Extract(link)
	if err != nil {
		t.Fatalf("Extract() failed: %v", err)
	}

	if extracted != longQuery {
		t.Error("Round trip failed for long query")
	}
}

func TestBuildSpecialCharactersInClusterAndDatabase(t *testing.T) {
	// Test with special characters that need URL encoding
	link, err := Build("print 1", "cluster/with/slashes", "database with spaces", "")
	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}

	// Verify the cluster and database are properly encoded
	if !strings.Contains(link, "cluster%2Fwith%2Fslashes") {
		t.Errorf("Build() did not properly encode cluster: %s", link)
	}
	if !strings.Contains(link, "database%20with%20spaces") {
		t.Errorf("Build() did not properly encode database: %s", link)
	}
}


