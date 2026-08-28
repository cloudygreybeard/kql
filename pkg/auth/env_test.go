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
	"errors"
	"strconv"
	"testing"
	"time"
)

// makeJWT builds an unsigned token carrying the given expiry claim.
func makeJWT(exp int64) string {
	encode := func(s string) string {
		return base64.RawURLEncoding.EncodeToString([]byte(s))
	}
	payload := `{"exp":` + strconv.FormatInt(exp, 10) + `}`
	return encode(`{"alg":"none"}`) + "." + encode(payload) + ".signature"
}

// newTestEnv returns an Env reading from the supplied values.
func newTestEnv(values map[string]string, now time.Time) *Env {
	return &Env{
		lookup: func(key string) (string, bool) {
			v, ok := values[key]
			return v, ok
		},
		now: func() time.Time { return now },
	}
}

func TestEnvUnset(t *testing.T) {
	e := newTestEnv(nil, time.Now())

	_, err := e.Token(context.Background(), "resource")
	if !errors.Is(err, ErrNotAvailable) {
		t.Fatalf("Env.Token() error = %v, want ErrNotAvailable", err)
	}
}

func TestEnvBlank(t *testing.T) {
	e := newTestEnv(map[string]string{TokenEnvVar: "   "}, time.Now())

	_, err := e.Token(context.Background(), "resource")
	if !errors.Is(err, ErrNotAvailable) {
		t.Fatalf("Env.Token() error = %v, want ErrNotAvailable", err)
	}
}

func TestEnvOpaqueToken(t *testing.T) {
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	e := newTestEnv(map[string]string{TokenEnvVar: " opaque "}, now)

	got, err := e.Token(context.Background(), "resource")
	if err != nil {
		t.Fatalf("Env.Token() returned unexpected error: %v", err)
	}
	if got.Value != "opaque" {
		t.Errorf("Env.Token().Value = %q, want %q", got.Value, "opaque")
	}
	want := now.Add(assumedTokenLifetime)
	if !got.ExpiresAt.Equal(want) {
		t.Errorf("Env.Token().ExpiresAt = %v, want %v", got.ExpiresAt, want)
	}
}

func TestEnvReadsJWTExpiry(t *testing.T) {
	exp := time.Date(2026, 8, 25, 13, 0, 0, 0, time.UTC).Unix()
	e := newTestEnv(map[string]string{TokenEnvVar: makeJWT(exp)}, time.Now())

	got, err := e.Token(context.Background(), "resource")
	if err != nil {
		t.Fatalf("Env.Token() returned unexpected error: %v", err)
	}
	if got.ExpiresAt.Unix() != exp {
		t.Errorf("Env.Token().ExpiresAt = %v, want %v", got.ExpiresAt.Unix(), exp)
	}
}

func TestJWTExpiry(t *testing.T) {
	tests := []struct {
		name  string
		token string
		want  bool
	}{
		{name: "not a jwt", token: "opaque", want: false},
		{name: "wrong segment count", token: "a.b", want: false},
		{name: "payload not base64", token: "a.!!!.c", want: false},
		{name: "payload not json", token: "YQ.YQ.YQ", want: false},
		{name: "valid", token: makeJWT(1756258712), want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, got := jwtExpiry(tt.token)
			if got != tt.want {
				t.Errorf("jwtExpiry(%q) ok = %v, want %v", tt.token, got, tt.want)
			}
		})
	}
}
