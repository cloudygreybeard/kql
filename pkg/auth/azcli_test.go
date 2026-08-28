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

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"
)

// newTestAzureCLI returns a provider whose command lookup and execution are
// controlled by the test.
func newTestAzureCLI(installed bool, run runner) *AzureCLI {
	return &AzureCLI{
		run: run,
		lookPath: func(name string) (string, error) {
			if !installed {
				return "", errors.New("not found")
			}
			return "/usr/bin/" + name, nil
		},
	}
}

func TestAzureCLISpec(t *testing.T) {
	a := newTestAzureCLI(true, nil)

	got, err := a.spec("https://cluster.kusto.windows.net")
	if err != nil {
		t.Fatalf("spec() returned unexpected error: %v", err)
	}
	if got.name != azCommand {
		t.Errorf("spec().name = %q, want %q", got.name, azCommand)
	}
	if !strings.Contains(got.String(), "get-access-token") {
		t.Errorf("spec() = %q, want it to invoke get-access-token", got)
	}
	if !strings.Contains(got.String(), "https://cluster.kusto.windows.net") {
		t.Errorf("spec() = %q, want it to carry the resource", got)
	}
}

func TestAzureCLISpecWithoutTheCLI(t *testing.T) {
	a := newTestAzureCLI(false, nil)

	// A missing Azure CLI must be reported as unavailable rather than as a
	// failure, so that a chain can move on to another provider.
	if _, err := a.spec("https://cluster.kusto.windows.net"); !errors.Is(err, ErrNotAvailable) {
		t.Fatalf("spec() error = %v, want ErrNotAvailable", err)
	}
}

func TestAzureCLISpecIncludesTenant(t *testing.T) {
	a := newTestAzureCLI(true, nil)
	a.Tenant = "contoso.example"

	got, err := a.spec("https://cluster.kusto.windows.net")
	if err != nil {
		t.Fatalf("spec() returned unexpected error: %v", err)
	}
	if !strings.Contains(got.String(), "--tenant contoso.example") {
		t.Errorf("spec() = %q, want it to carry the tenant", got)
	}
}

func TestAzureCLIToken(t *testing.T) {
	expiry := time.Now().Add(time.Hour).Unix()
	run := func(context.Context, commandSpec) ([]byte, error) {
		return []byte(`{"accessToken":"secret","expires_on":` +
			strconv.FormatInt(expiry, 10) + `,"tokenType":"Bearer"}`), nil
	}

	a := newTestAzureCLI(true, run)

	got, err := a.Token(context.Background(), "https://cluster.kusto.windows.net")
	if err != nil {
		t.Fatalf("Token() returned unexpected error: %v", err)
	}
	if got.Value != "secret" {
		t.Errorf("Token().Value = %q, want %q", got.Value, "secret")
	}
	if got.ExpiresAt.Unix() != expiry {
		t.Errorf("Token().ExpiresAt = %v, want %v", got.ExpiresAt.Unix(), expiry)
	}
}

func TestAzureCLITokenEmptyResource(t *testing.T) {
	a := newTestAzureCLI(true, nil)
	if _, err := a.Token(context.Background(), ""); err == nil {
		t.Fatal("Token() succeeded, want error")
	}
}

func TestAzureCLITokenReportsLoginHint(t *testing.T) {
	run := func(context.Context, commandSpec) ([]byte, error) {
		return nil, errors.New("please run az login")
	}
	a := newTestAzureCLI(true, run)

	_, err := a.Token(context.Background(), "https://cluster.kusto.windows.net")
	if err == nil {
		t.Fatal("Token() succeeded, want error")
	}
	if !strings.Contains(err.Error(), "az login") {
		t.Errorf("Token() error = %q, want a login hint", err)
	}
	if !strings.Contains(err.Error(), "auth_command") {
		t.Errorf("Token() error = %q, want the helper alternative mentioned", err)
	}
}

func TestParseToken(t *testing.T) {
	local := time.Date(2026, 8, 27, 1, 38, 32, 0, time.Local)

	tests := []struct {
		name    string
		input   string
		want    time.Time
		wantErr bool
	}{
		{
			name:  "unix expiry is preferred",
			input: `{"accessToken":"a","expires_on":1756258712,"expiresOn":"nonsense"}`,
			want:  time.Unix(1756258712, 0),
		},
		{
			name:  "unix expiry as a string",
			input: `{"accessToken":"a","expires_on":"1756258712"}`,
			want:  time.Unix(1756258712, 0),
		},
		{
			name:  "local expiry with microseconds",
			input: `{"accessToken":"a","expiresOn":"2026-08-27 01:38:32.000000"}`,
			want:  local,
		},
		{
			name:  "local expiry without microseconds",
			input: `{"accessToken":"a","expiresOn":"2026-08-27 01:38:32"}`,
			want:  local,
		},
		{
			name:  "leading warning is tolerated",
			input: "warning: something happened\n" + `{"accessToken":"a","expires_on":1756258712}`,
			want:  time.Unix(1756258712, 0),
		},
		{
			name:    "missing access token",
			input:   `{"expires_on":1756258712}`,
			wantErr: true,
		},
		{
			name:    "unparseable expiry",
			input:   `{"accessToken":"a","expiresOn":"whenever"}`,
			wantErr: true,
		},
		{
			name:    "not json",
			input:   "command not found",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseToken([]byte(tt.input))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseToken(%q) succeeded, want error", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseToken(%q) returned unexpected error: %v", tt.input, err)
			}
			if !got.ExpiresAt.Equal(tt.want) {
				t.Errorf("parseToken(%q).ExpiresAt = %v, want %v", tt.input, got.ExpiresAt, tt.want)
			}
		})
	}
}
