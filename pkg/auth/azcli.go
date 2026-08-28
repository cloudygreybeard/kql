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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// commandTimeout bounds a single credential command. It accommodates a cold
// Azure CLI start, and an external helper that prompts a broker or a hardware
// token.
const commandTimeout = 90 * time.Second

// azCommand is the Azure CLI executable, as found on PATH.
const azCommand = "az"

// commandSpec describes a single Azure CLI invocation.
type commandSpec struct {
	name string
	args []string
}

// String renders the invocation for diagnostic output.
func (s commandSpec) String() string {
	return strings.TrimSpace(s.name + " " + strings.Join(s.args, " "))
}

// runner executes spec and returns its standard output.
type runner func(ctx context.Context, spec commandSpec) ([]byte, error)

// AzureCLI obtains tokens by invoking "az account get-access-token".
//
// The Azure CLI must already hold a session for the same operating system
// process space, established with "az login". Where that is impractical, or
// where tokens come from elsewhere entirely, [Exec] runs a helper of the
// caller's choosing instead.
type AzureCLI struct {
	// Tenant optionally overrides the tenant a token is requested for.
	Tenant string

	// run executes the Azure CLI. Tests replace it.
	run runner
	// lookPath reports whether the Azure CLI is installed. Tests replace it.
	lookPath func(string) (string, error)
}

// NewAzureCLI returns an AzureCLI provider for the given tenant. An empty
// tenant uses the Azure CLI's current tenant.
func NewAzureCLI(tenant string) *AzureCLI {
	return &AzureCLI{
		Tenant:   tenant,
		run:      execRunner,
		lookPath: exec.LookPath,
	}
}

// Name identifies the provider in diagnostic output.
func (a *AzureCLI) Name() string { return "azure-cli" }

// Token returns a bearer token for resource obtained from the Azure CLI.
func (a *AzureCLI) Token(ctx context.Context, resource string) (Token, error) {
	if resource == "" {
		return Token{}, fmt.Errorf("resource must not be empty")
	}

	spec, err := a.spec(resource)
	if err != nil {
		return Token{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()

	out, err := a.run(ctx, spec)
	if err != nil {
		return Token{}, fmt.Errorf("%s failed: %w\n%s", spec, err, loginHint())
	}

	tok, err := parseToken(out)
	if err != nil {
		return Token{}, fmt.Errorf("%s: %w", spec, err)
	}
	return tok, nil
}

// spec builds the Azure CLI invocation for resource, reporting
// [ErrNotAvailable] when the Azure CLI is not installed.
func (a *AzureCLI) spec(resource string) (commandSpec, error) {
	if _, err := a.lookPath(azCommand); err != nil {
		return commandSpec{}, fmt.Errorf("%w: %s not found on PATH", ErrNotAvailable, azCommand)
	}

	args := []string{"account", "get-access-token", "--resource", resource, "--output", "json"}
	if a.Tenant != "" {
		args = append(args, "--tenant", a.Tenant)
	}
	return commandSpec{name: azCommand, args: args}, nil
}

// loginHint suggests how to obtain a session.
func loginHint() string {
	return "hint: run 'az login', or set query.auth_command to obtain tokens from an external helper"
}

// execRunner runs spec, returning its standard output. Standard error is
// folded into the returned error so that Azure CLI diagnostics reach the user.
func execRunner(ctx context.Context, spec commandSpec) ([]byte, error) {
	cmd := exec.CommandContext(ctx, spec.name, spec.args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return nil, fmt.Errorf("%w: %s", err, msg)
		}
		return nil, err
	}
	return stdout.Bytes(), nil
}

// azTokenResponse mirrors the JSON emitted by "az account get-access-token".
// It is also the response shape [Exec] accepts, so that a helper wrapping the
// Azure CLI needs no adaptation.
type azTokenResponse struct {
	AccessToken string `json:"accessToken"`
	ExpiresOn   string `json:"expiresOn"`
	ExpiresOnU  any    `json:"expires_on"`
}

// expiryLayouts are the layouts the Azure CLI has used for "expiresOn". None
// carries a zone, so each is interpreted as local time.
var expiryLayouts = []string{
	"2006-01-02 15:04:05.000000",
	"2006-01-02 15:04:05",
	time.RFC3339,
}

// parseToken extracts a Token from Azure CLI output.
func parseToken(out []byte) (Token, error) {
	body, err := extractJSONObject(out)
	if err != nil {
		return Token{}, err
	}

	var resp azTokenResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return Token{}, fmt.Errorf("parsing token response: %w", err)
	}
	if resp.AccessToken == "" {
		return Token{}, fmt.Errorf("token response contained no access token")
	}

	expiry, err := tokenExpiry(resp)
	if err != nil {
		return Token{}, err
	}
	return Token{Value: resp.AccessToken, ExpiresAt: expiry}, nil
}

// tokenExpiry determines when a token lapses, preferring the unambiguous Unix
// "expires_on" field over the zoneless local "expiresOn" string.
func tokenExpiry(resp azTokenResponse) (time.Time, error) {
	if secs, ok := unixSeconds(resp.ExpiresOnU); ok {
		return time.Unix(secs, 0), nil
	}

	value := strings.TrimSpace(resp.ExpiresOn)
	for _, layout := range expiryLayouts {
		if t, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognised token expiry %q", resp.ExpiresOn)
}

// unixSeconds interprets the "expires_on" field, which the Azure CLI has
// emitted as both a JSON number and a string.
func unixSeconds(v any) (int64, bool) {
	switch n := v.(type) {
	case float64:
		return int64(n), true
	case string:
		secs, err := strconv.ParseInt(strings.TrimSpace(n), 10, 64)
		return secs, err == nil
	default:
		return 0, false
	}
}

// extractJSONObject returns the first JSON object in out.
//
// A credential command may print warnings on standard output before its own
// output, so the object is located rather than assumed to start at the first
// byte.
func extractJSONObject(out []byte) ([]byte, error) {
	start := bytes.IndexByte(out, '{')
	end := bytes.LastIndexByte(out, '}')
	if start < 0 || end < start {
		return nil, fmt.Errorf("no JSON object in Azure CLI output: %q", truncate(out, 200))
	}
	return out[start : end+1], nil
}

// truncate shortens b for inclusion in an error message.
func truncate(b []byte, n int) string {
	s := strings.TrimSpace(string(b))
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
