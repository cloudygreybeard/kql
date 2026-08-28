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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// TokenEnvVar names an environment variable holding a bearer token to use in
// preference to the Azure CLI.
//
// It suits continuous integration, and tight scripted loops where repeatedly
// invoking the Azure CLI would dominate runtime. Callers should be aware that
// environment variables are visible to other processes owned by the same user.
const TokenEnvVar = "KQL_TOKEN"

// assumedTokenLifetime is how long a supplied token is assumed to remain
// valid when its expiry cannot be determined.
const assumedTokenLifetime = time.Hour

// Env supplies a bearer token taken from the environment.
type Env struct {
	// lookup reads an environment variable. Tests replace it.
	lookup func(string) (string, bool)
	// now reports the current time. Tests replace it.
	now func() time.Time
}

// NewEnv returns a provider reading a token from [TokenEnvVar].
func NewEnv() *Env {
	return &Env{lookup: os.LookupEnv, now: time.Now}
}

// Name identifies the provider in diagnostic output.
func (e *Env) Name() string { return "env(" + TokenEnvVar + ")" }

// Token returns the token held in [TokenEnvVar], reporting [ErrNotAvailable]
// when the variable is unset or empty.
//
// The resource is not consulted: a caller supplying a token asserts that it
// carries the correct audience.
func (e *Env) Token(_ context.Context, _ string) (Token, error) {
	raw, ok := e.lookup(TokenEnvVar)
	if !ok || strings.TrimSpace(raw) == "" {
		return Token{}, fmt.Errorf("%w: %s is not set", ErrNotAvailable, TokenEnvVar)
	}

	value := strings.TrimSpace(raw)
	expiry, ok := jwtExpiry(value)
	if !ok {
		expiry = e.now().Add(assumedTokenLifetime)
	}
	return Token{Value: value, ExpiresAt: expiry}, nil
}

// jwtExpiry reads the "exp" claim of an unverified JSON Web Token.
//
// The signature is deliberately not checked. The claim is used only to decide
// when to stop reusing a token, a decision the issuing service enforces
// independently.
func jwtExpiry(token string) (time.Time, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return time.Time{}, false
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}, false
	}

	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil || claims.Exp == 0 {
		return time.Time{}, false
	}
	return time.Unix(claims.Exp, 0), true
}
