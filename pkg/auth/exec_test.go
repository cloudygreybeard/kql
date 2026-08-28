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
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"
)

const testResource = "https://cluster.kusto.windows.net"

// newTestExec returns a helper whose execution is controlled by the test.
func newTestExec(command []string, env map[string]string, run func(context.Context, helperCommand) ([]byte, error)) *Exec {
	e := NewExec(command, env)
	e.run = run
	return e
}

// staticHelper returns a runner producing out.
func staticHelper(out string) func(context.Context, helperCommand) ([]byte, error) {
	return func(context.Context, helperCommand) ([]byte, error) { return []byte(out), nil }
}

func TestExecUnconfigured(t *testing.T) {
	// An absent helper must be reported as unavailable, so that a chain moves
	// on to the Azure CLI rather than failing.
	_, err := NewExec(nil, nil).Token(context.Background(), testResource)
	if !errors.Is(err, ErrNotAvailable) {
		t.Fatalf("Token() error = %v, want ErrNotAvailable", err)
	}
}

func TestExecEmptyResource(t *testing.T) {
	e := newTestExec([]string{"helper"}, nil, staticHelper("token"))
	if _, err := e.Token(context.Background(), ""); err == nil {
		t.Fatal("Token() succeeded, want error")
	}
}

func TestExecAcceptsAzureCLIJSON(t *testing.T) {
	expiry := time.Now().Add(time.Hour).Unix()
	out := fmt.Sprintf(`{"accessToken":"secret","expires_on":%d,"tokenType":"Bearer"}`, expiry)
	e := newTestExec([]string{"helper"}, nil, staticHelper(out))

	got, err := e.Token(context.Background(), testResource)
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

func TestExecAcceptsBareToken(t *testing.T) {
	exp := time.Now().Add(30 * time.Minute).Unix()
	token := makeJWT(exp)
	e := newTestExec([]string{"helper"}, nil, staticHelper(token+"\n"))

	got, err := e.Token(context.Background(), testResource)
	if err != nil {
		t.Fatalf("Token() returned unexpected error: %v", err)
	}
	if got.Value != token {
		t.Errorf("Token().Value = %q, want the token verbatim", got.Value)
	}
	if got.ExpiresAt.Unix() != exp {
		t.Errorf("Token().ExpiresAt = %v, want %v", got.ExpiresAt.Unix(), exp)
	}
}

func TestExecBareTokenWithoutExpiry(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	e := newTestExec([]string{"helper"}, nil, staticHelper("opaque-token"))
	e.now = func() time.Time { return now }

	got, err := e.Token(context.Background(), testResource)
	if err != nil {
		t.Fatalf("Token() returned unexpected error: %v", err)
	}
	if want := now.Add(assumedTokenLifetime); !got.ExpiresAt.Equal(want) {
		t.Errorf("Token().ExpiresAt = %v, want %v", got.ExpiresAt, want)
	}
}

func TestExecRejectsUnusableOutput(t *testing.T) {
	tests := []struct {
		name  string
		out   string
		wants string
	}{
		{name: "empty", out: "   \n", wants: "no output"},
		{name: "prose", out: "error: not logged in\n", wants: "bare token"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newTestExec([]string{"helper"}, nil, staticHelper(tt.out))

			_, err := e.Token(context.Background(), testResource)
			if err == nil {
				t.Fatalf("Token() succeeded, want error")
			}
			if !strings.Contains(err.Error(), tt.wants) {
				t.Errorf("Token() error = %q, want it to mention %q", err, tt.wants)
			}
		})
	}
}

func TestExecPassesResourceInEnvironment(t *testing.T) {
	var got []string
	run := func(_ context.Context, c helperCommand) ([]byte, error) {
		got = c.env
		return []byte("token"), nil
	}
	// A configured variable of the same name must not displace the audience,
	// which the helper is required to honour.
	e := newTestExec([]string{"helper"}, map[string]string{
		"EXTRA":        "value",
		ResourceEnvVar: "wrong",
	}, run)

	if _, err := e.Token(context.Background(), testResource); err != nil {
		t.Fatalf("Token() returned unexpected error: %v", err)
	}
	if want := ResourceEnvVar + "=" + testResource; lastValue(got, ResourceEnvVar) != want {
		t.Errorf("last %s entry = %q, want %q", ResourceEnvVar, lastValue(got, ResourceEnvVar), want)
	}
	if want := "EXTRA=value"; lastValue(got, "EXTRA") != want {
		t.Errorf("EXTRA entry = %q, want %q", lastValue(got, "EXTRA"), want)
	}
}

// lastValue returns the final assignment of name in env, which is the one the
// operating system applies.
func lastValue(env []string, name string) string {
	found := ""
	for _, entry := range env {
		if strings.HasPrefix(entry, name+"=") {
			found = entry
		}
	}
	return found
}

func TestExecReportsRedactedDiagnostics(t *testing.T) {
	secret := makeJWT(time.Now().Add(time.Hour).Unix())
	run := func(context.Context, helperCommand) ([]byte, error) {
		return nil, fmt.Errorf("exit status 1: cached %s is stale", redact("cached "+secret+" is stale"))
	}
	e := newTestExec([]string{"helper"}, nil, run)

	_, err := e.Token(context.Background(), testResource)
	if err == nil {
		t.Fatal("Token() succeeded, want error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Errorf("Token() error disclosed a token: %q", err)
	}
}

func TestRedact(t *testing.T) {
	token := makeJWT(1756258712)
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "full token", input: "using " + token, want: "using [redacted]"},
		{name: "header only", input: "eyJhbGciOiJub25lIn0", want: "[redacted]"},
		{name: "unrelated text", input: "az login failed", want: "az login failed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := redact(tt.input); got != tt.want {
				t.Errorf("redact(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestExecRunsWithoutAShell(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the test relies on a POSIX shell and utilities")
	}

	// Were the arguments handed to a shell, the variable would be expanded.
	// Run directly, it reaches the helper verbatim.
	e := NewExec([]string{"/bin/echo", "$" + ResourceEnvVar}, nil)

	got, err := e.Token(context.Background(), testResource)
	if err != nil {
		t.Fatalf("Token() returned unexpected error: %v", err)
	}
	if want := "$" + ResourceEnvVar; got.Value != want {
		t.Errorf("Token().Value = %q, want %q, so the arguments were interpreted", got.Value, want)
	}
}

func TestExecHelperReadsTheEnvironment(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the test relies on a POSIX shell and utilities")
	}

	// A helper is free to use a shell of its own choosing; kql simply does not
	// impose one.
	e := NewExec([]string{"/bin/sh", "-c", `printf %s "$` + ResourceEnvVar + `"`}, nil)

	got, err := e.Token(context.Background(), testResource)
	if err != nil {
		t.Fatalf("Token() returned unexpected error: %v", err)
	}
	if got.Value != testResource {
		t.Errorf("Token().Value = %q, want %q", got.Value, testResource)
	}
}

func TestExecReportsFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the test relies on a POSIX shell and utilities")
	}

	e := NewExec([]string{"/bin/sh", "-c", "echo 'no session' >&2; exit 3"}, nil)

	_, err := e.Token(context.Background(), testResource)
	if err == nil {
		t.Fatal("Token() succeeded, want error")
	}
	if !strings.Contains(err.Error(), "no session") {
		t.Errorf("Token() error = %q, want the helper's diagnostics", err)
	}
}

func TestExecName(t *testing.T) {
	tests := []struct {
		name    string
		command []string
		want    string
	}{
		{name: "unconfigured", command: nil, want: "exec"},
		{name: "path is reduced to its base", command: []string{"/opt/bin/broker", "--x"}, want: "exec(broker)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewExec(tt.command, nil).Name(); got != tt.want {
				t.Errorf("Name() = %q, want %q", got, tt.want)
			}
		})
	}
}
