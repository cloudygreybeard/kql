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

package auth

import "testing"

func TestParseSource(t *testing.T) {
	tests := []struct {
		input   string
		want    Source
		wantErr bool
	}{
		{input: "", want: SourceAuto},
		{input: "auto", want: SourceAuto},
		{input: "AUTO", want: SourceAuto},
		{input: "  auto  ", want: SourceAuto},
		{input: "az", want: SourceAzureCLI},
		{input: "azure-cli", want: SourceAzureCLI},
		{input: "exec", want: SourceExec},
		{input: "command", want: SourceExec},
		{input: "env", want: SourceEnv},
		{input: "nonsense", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseSource(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseSource(%q) succeeded, want error", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseSource(%q) returned unexpected error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("ParseSource(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestSourceStringRoundTrips(t *testing.T) {
	for _, want := range []Source{SourceAuto, SourceAzureCLI, SourceExec, SourceEnv} {
		got, err := ParseSource(want.String())
		if err != nil {
			t.Fatalf("ParseSource(%q) returned unexpected error: %v", want, err)
		}
		if got != want {
			t.Errorf("ParseSource(%q) = %v, want %v", want.String(), got, want)
		}
	}
}
