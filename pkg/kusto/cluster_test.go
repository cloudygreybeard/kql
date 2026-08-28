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

import "testing"

func TestResolveCluster(t *testing.T) {
	aliases := map[string]string{
		"prod-eu": "mycluster.westeurope",
		"samples": "https://help.kusto.windows.net",
		"blank":   "   ",
	}

	tests := []struct {
		name    string
		ref     string
		want    string
		wantErr bool
	}{
		{
			name: "short name gains the default domain",
			ref:  "help",
			want: "https://help.kusto.windows.net",
		},
		{
			name: "regional short name gains the default domain",
			ref:  "mycluster.westeurope",
			want: "https://mycluster.westeurope.kusto.windows.net",
		},
		{
			name: "complete host is left alone",
			ref:  "mycluster.westeurope.kusto.windows.net",
			want: "https://mycluster.westeurope.kusto.windows.net",
		},
		{
			name: "sovereign host is left alone",
			ref:  "mycluster.kusto.chinacloudapi.cn",
			want: "https://mycluster.kusto.chinacloudapi.cn",
		},
		{
			name: "absolute url is preserved",
			ref:  "https://help.kusto.windows.net",
			want: "https://help.kusto.windows.net",
		},
		{
			name: "url path is discarded",
			ref:  "https://help.kusto.windows.net/v2/rest/query",
			want: "https://help.kusto.windows.net",
		},
		{
			name: "port is preserved",
			ref:  "http://localhost:8080",
			want: "http://localhost:8080",
		},
		{
			name: "surrounding space is ignored",
			ref:  "  help  ",
			want: "https://help.kusto.windows.net",
		},
		{
			name: "alias to a short name",
			ref:  "prod-eu",
			want: "https://mycluster.westeurope.kusto.windows.net",
		},
		{
			name: "alias to a url",
			ref:  "samples",
			want: "https://help.kusto.windows.net",
		},
		{
			name:    "empty reference",
			ref:     "",
			wantErr: true,
		},
		{
			name:    "unsupported scheme",
			ref:     "ftp://example.net",
			wantErr: true,
		},
		{
			name:    "alias resolving to nothing",
			ref:     "blank",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveCluster(tt.ref, aliases)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ResolveCluster(%q) = %q, want error", tt.ref, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveCluster(%q) returned unexpected error: %v", tt.ref, err)
			}
			if got != tt.want {
				t.Errorf("ResolveCluster(%q) = %q, want %q", tt.ref, got, tt.want)
			}
		})
	}
}

func TestResolveClusterWithoutAliases(t *testing.T) {
	got, err := ResolveCluster("help", nil)
	if err != nil {
		t.Fatalf("ResolveCluster() returned unexpected error: %v", err)
	}
	if want := "https://help.kusto.windows.net"; got != want {
		t.Errorf("ResolveCluster(%q, nil) = %q, want %q", "help", got, want)
	}
}

func TestResource(t *testing.T) {
	tests := []struct {
		endpoint string
		want     string
	}{
		{endpoint: "https://help.kusto.windows.net", want: "https://help.kusto.windows.net"},
		{endpoint: "https://help.kusto.windows.net/", want: "https://help.kusto.windows.net"},
	}

	for _, tt := range tests {
		t.Run(tt.endpoint, func(t *testing.T) {
			if got := Resource(tt.endpoint); got != tt.want {
				t.Errorf("Resource(%q) = %q, want %q", tt.endpoint, got, tt.want)
			}
		})
	}
}
