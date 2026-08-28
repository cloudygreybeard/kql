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
	"testing"
	"time"
)

// stubProvider returns a fixed token or error, counting its invocations.
type stubProvider struct {
	name  string
	token Token
	err   error
	calls int
}

func (s *stubProvider) Name() string { return s.name }

func (s *stubProvider) Token(context.Context, string) (Token, error) {
	s.calls++
	if s.err != nil {
		return Token{}, s.err
	}
	return s.token, nil
}

func TestTokenValid(t *testing.T) {
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name   string
		token  Token
		margin time.Duration
		want   bool
	}{
		{
			name:  "empty value",
			token: Token{ExpiresAt: now.Add(time.Hour)},
			want:  false,
		},
		{
			name:  "expired",
			token: Token{Value: "t", ExpiresAt: now.Add(-time.Second)},
			want:  false,
		},
		{
			name:   "within margin",
			token:  Token{Value: "t", ExpiresAt: now.Add(time.Minute)},
			margin: 5 * time.Minute,
			want:   false,
		},
		{
			name:   "beyond margin",
			token:  Token{Value: "t", ExpiresAt: now.Add(time.Hour)},
			margin: 5 * time.Minute,
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.token.Valid(now, tt.margin); got != tt.want {
				t.Errorf("Token.Valid(%v, %v) = %v, want %v", now, tt.margin, got, tt.want)
			}
		})
	}
}

func TestChainSkipsUnavailableProviders(t *testing.T) {
	unavailable := &stubProvider{name: "first", err: ErrNotAvailable}
	available := &stubProvider{name: "second", token: Token{Value: "token"}}

	got, err := NewChain(unavailable, available).Token(context.Background(), "resource")
	if err != nil {
		t.Fatalf("Chain.Token() returned unexpected error: %v", err)
	}
	if got.Value != "token" {
		t.Errorf("Chain.Token() = %q, want %q", got.Value, "token")
	}
	if unavailable.calls != 1 {
		t.Errorf("first provider called %d times, want 1", unavailable.calls)
	}
}

func TestChainStopsOnRealError(t *testing.T) {
	failing := &stubProvider{name: "first", err: errors.New("no session")}
	later := &stubProvider{name: "second", token: Token{Value: "token"}}

	_, err := NewChain(failing, later).Token(context.Background(), "resource")
	if err == nil {
		t.Fatal("Chain.Token() succeeded, want error")
	}
	if later.calls != 0 {
		t.Errorf("second provider called %d times, want 0", later.calls)
	}
}

func TestChainExhausted(t *testing.T) {
	_, err := NewChain(&stubProvider{name: "only", err: ErrNotAvailable}).
		Token(context.Background(), "resource")
	if err == nil {
		t.Fatal("Chain.Token() succeeded, want error")
	}
}

func TestChainEmpty(t *testing.T) {
	if _, err := NewChain().Token(context.Background(), "resource"); err == nil {
		t.Fatal("Chain.Token() succeeded, want error")
	}
}

func TestCacheReusesValidToken(t *testing.T) {
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	provider := &stubProvider{name: "stub", token: Token{Value: "token", ExpiresAt: now.Add(time.Hour)}}

	cache := NewCache(provider)
	cache.now = func() time.Time { return now }

	for i := 0; i < 3; i++ {
		if _, err := cache.Token(context.Background(), "resource"); err != nil {
			t.Fatalf("Cache.Token() returned unexpected error: %v", err)
		}
	}
	if provider.calls != 1 {
		t.Errorf("provider called %d times, want 1", provider.calls)
	}
}

func TestCacheRefreshesExpiredToken(t *testing.T) {
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	provider := &stubProvider{name: "stub", token: Token{Value: "token", ExpiresAt: now.Add(time.Minute)}}

	cache := NewCache(provider)
	cache.now = func() time.Time { return now }

	for i := 0; i < 2; i++ {
		if _, err := cache.Token(context.Background(), "resource"); err != nil {
			t.Fatalf("Cache.Token() returned unexpected error: %v", err)
		}
	}
	// The token expires inside ExpiryMargin, so it must not be reused.
	if provider.calls != 2 {
		t.Errorf("provider called %d times, want 2", provider.calls)
	}
}

func TestCacheKeysByResource(t *testing.T) {
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	provider := &stubProvider{name: "stub", token: Token{Value: "token", ExpiresAt: now.Add(time.Hour)}}

	cache := NewCache(provider)
	cache.now = func() time.Time { return now }

	for _, resource := range []string{"one", "two", "one"} {
		if _, err := cache.Token(context.Background(), resource); err != nil {
			t.Fatalf("Cache.Token(%q) returned unexpected error: %v", resource, err)
		}
	}
	if provider.calls != 2 {
		t.Errorf("provider called %d times, want 2", provider.calls)
	}
}
