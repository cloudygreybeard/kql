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
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// ResourceEnvVar names the environment variable through which a credential
// helper is told which audience to obtain a token for.
//
// The audience is passed in the environment rather than substituted into the
// helper's arguments. Interpolating a value into a command line invites the
// injection defects that have repeatedly troubled tools accepting a command
// as a single string.
const ResourceEnvVar = "KQL_AUTH_RESOURCE"

// maxHelperDiagnostic bounds how much helper standard error is quoted in an
// error message.
const maxHelperDiagnostic = 2000

// Exec obtains tokens by running a credential helper nominated in the
// configuration file as query.auth_command.
//
// The helper is run directly, never through a shell, and its arguments are
// used verbatim: kql performs no substitution, quoting or word splitting on
// them. The audience is supplied in [ResourceEnvVar], and any further
// variables the helper needs are passed through unaltered from configuration.
//
// The helper writes a token on standard output, either as the JSON object
// "az account get-access-token" emits, or as a bare token on a single line.
// Standard input is not inherited, since kql may itself be reading the query
// from it.
type Exec struct {
	// Command is the helper and its arguments. The first element is the
	// executable, resolved against PATH when it is not a path.
	Command []string
	// Env holds additional environment variables for the helper.
	Env map[string]string

	// run executes the helper. Tests replace it.
	run func(ctx context.Context, c helperCommand) ([]byte, error)
	// now reports the current time. Tests replace it.
	now func() time.Time
}

// NewExec returns a provider running command with env in addition to the
// inherited environment. An empty command yields a provider reporting
// [ErrNotAvailable].
func NewExec(command []string, env map[string]string) *Exec {
	return &Exec{Command: command, Env: env, run: runHelper, now: time.Now}
}

// Name identifies the provider in diagnostic output. Only the helper's base
// name is used, since its arguments are not necessarily fit to display.
func (e *Exec) Name() string {
	if len(e.Command) == 0 {
		return "exec"
	}
	return "exec(" + filepath.Base(e.Command[0]) + ")"
}

// Token returns a bearer token for resource obtained from the helper.
//
// A helper that is configured but cannot be run is an error rather than
// [ErrNotAvailable], so that a broken helper is reported instead of being
// silently passed over in favour of another provider.
func (e *Exec) Token(ctx context.Context, resource string) (Token, error) {
	if len(e.Command) == 0 {
		return Token{}, fmt.Errorf("%w: no credential helper configured", ErrNotAvailable)
	}
	if resource == "" {
		return Token{}, errors.New("resource must not be empty")
	}

	ctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()

	out, err := e.run(ctx, helperCommand{argv: e.Command, env: e.environ(resource)})
	if err != nil {
		return Token{}, fmt.Errorf("credential helper %s failed: %w", e.Command[0], err)
	}

	tok, err := parseHelperToken(out, e.now())
	if err != nil {
		return Token{}, fmt.Errorf("credential helper %s: %w", e.Command[0], err)
	}
	return tok, nil
}

// environ builds the helper's environment. Configured variables are applied
// over the inherited environment, and the audience last, so that it cannot be
// shadowed.
func (e *Exec) environ(resource string) []string {
	env := os.Environ()

	names := make([]string, 0, len(e.Env))
	for name := range e.Env {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		env = append(env, name+"="+e.Env[name])
	}

	return append(env, ResourceEnvVar+"="+resource)
}

// helperCommand describes a single credential helper invocation.
type helperCommand struct {
	argv []string
	env  []string
}

// runHelper runs c and returns its standard output.
func runHelper(ctx context.Context, c helperCommand) ([]byte, error) {
	cmd := exec.CommandContext(ctx, c.argv[0], c.argv[1:]...)
	cmd.Env = c.env
	// kql may be reading the query from standard input, so the helper is
	// given none rather than allowed to consume it.
	cmd.Stdin = nil

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if msg := redact(truncate(stderr.Bytes(), maxHelperDiagnostic)); msg != "" {
			return nil, fmt.Errorf("%w: %s", err, msg)
		}
		return nil, err
	}
	return stdout.Bytes(), nil
}

// parseHelperToken reads a token from helper output, accepting either the
// Azure CLI's JSON object or a bare token on a single line.
func parseHelperToken(out []byte, now time.Time) (Token, error) {
	if bytes.ContainsRune(out, '{') {
		return parseToken(out)
	}

	value := strings.TrimSpace(string(out))
	if value == "" {
		return Token{}, errors.New("produced no output")
	}
	if strings.ContainsAny(value, " \t\r\n") {
		return Token{}, fmt.Errorf("output is neither a JSON object nor a bare token: %s",
			redact(truncate([]byte(value), 120)))
	}

	expiry, ok := jwtExpiry(value)
	if !ok {
		expiry = now.Add(assumedTokenLifetime)
	}
	return Token{Value: value, ExpiresAt: expiry}, nil
}

// bearerPattern matches a base64url-encoded token of the kind carried in an
// Authorization header, whose payload begins with an encoded "{".
var bearerPattern = regexp.MustCompile(`eyJ[A-Za-z0-9_-]{8,}(\.[A-Za-z0-9_-]+){0,2}`)

// redact removes anything resembling a bearer token from s, so that helper
// diagnostics can be shown without disclosing a credential.
func redact(s string) string {
	return bearerPattern.ReplaceAllString(s, "[redacted]")
}
