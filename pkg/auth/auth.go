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

// Package auth obtains Microsoft Entra ID bearer tokens for Kusto data-plane
// requests.
//
// kql stores no credentials of its own. Tokens are obtained by default from
// the Azure CLI through its documented "az account get-access-token"
// interface, which leaves token caching, refresh and Conditional Access
// enforcement to the Azure CLI. Where that does not suit, [Exec] runs a
// credential helper of the operator's choosing, and [Env] accepts a token
// supplied directly.
//
// The MSAL token cache beneath ~/.azure is deliberately never read. It is an
// internal implementation detail of the Azure CLI rather than a supported
// interface, its representation differs by platform, and it holds refresh
// tokens, which are longer lived and broader in scope than the access tokens
// kql needs. Shelling out costs a few seconds per invocation and leaves the
// stored-credential attack surface entirely with the Azure CLI.
package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// ExpiryMargin is how long before its stated expiry a token is treated as
// stale, so that a token is not presented to a server moments before it
// lapses.
const ExpiryMargin = 5 * time.Minute

// ErrNotAvailable indicates that a provider cannot operate in this
// environment, for instance because the Azure CLI is not installed. A provider
// returning it is skipped by [Chain].
var ErrNotAvailable = errors.New("credential provider not available")

// Token is a bearer token and the instant at which it expires.
type Token struct {
	// Value is the raw bearer token.
	Value string
	// ExpiresAt is when the token stops being accepted.
	ExpiresAt time.Time
}

// Valid reports whether t is non-empty and does not expire within margin of
// now.
func (t Token) Valid(now time.Time, margin time.Duration) bool {
	return t.Value != "" && t.ExpiresAt.After(now.Add(margin))
}

// Provider obtains bearer tokens for a resource, which is a Microsoft Entra ID
// audience such as "https://help.kusto.windows.net".
type Provider interface {
	// Token returns a bearer token for resource.
	Token(ctx context.Context, resource string) (Token, error)
	// Name identifies the provider in diagnostic output.
	Name() string
}

// Chain queries each provider in turn and returns the first token obtained.
// Providers reporting [ErrNotAvailable] are skipped; any other error stops the
// chain, so that a genuine authentication failure is reported rather than
// masked by a later provider.
type Chain struct {
	providers []Provider
}

// NewChain returns a Chain over providers, which are consulted in order.
func NewChain(providers ...Provider) *Chain {
	return &Chain{providers: providers}
}

// Name returns the names of the chained providers.
func (c *Chain) Name() string {
	names := make([]string, 0, len(c.providers))
	for _, p := range c.providers {
		names = append(names, p.Name())
	}
	return "chain(" + strings.Join(names, ", ") + ")"
}

// Token returns a token from the first provider able to supply one.
func (c *Chain) Token(ctx context.Context, resource string) (Token, error) {
	if len(c.providers) == 0 {
		return Token{}, errors.New("no credential providers configured")
	}

	var skipped []string
	for _, p := range c.providers {
		tok, err := p.Token(ctx, resource)
		if err == nil {
			return tok, nil
		}
		if errors.Is(err, ErrNotAvailable) {
			skipped = append(skipped, p.Name())
			continue
		}
		return Token{}, fmt.Errorf("%s: %w", p.Name(), err)
	}

	return Token{}, fmt.Errorf("no credential provider could supply a token (tried: %s)",
		strings.Join(skipped, ", "))
}

// Cache memoises tokens per resource for the lifetime of the process.
//
// Tokens are held in memory only and are never written to disk. Persisting
// them would duplicate the cache the Azure CLI already maintains while adding
// a second place for a bearer token to leak from.
type Cache struct {
	provider Provider
	margin   time.Duration
	now      func() time.Time

	mu     sync.Mutex
	tokens map[string]Token
}

// NewCache returns a Cache wrapping provider.
func NewCache(provider Provider) *Cache {
	return &Cache{
		provider: provider,
		margin:   ExpiryMargin,
		now:      time.Now,
		tokens:   make(map[string]Token),
	}
}

// Name returns the wrapped provider's name.
func (c *Cache) Name() string { return c.provider.Name() }

// Token returns a cached token for resource if one is still valid, and
// otherwise obtains a fresh token from the wrapped provider.
func (c *Cache) Token(ctx context.Context, resource string) (Token, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if tok, ok := c.tokens[resource]; ok && tok.Valid(c.now(), c.margin) {
		return tok, nil
	}

	tok, err := c.provider.Token(ctx, resource)
	if err != nil {
		return Token{}, err
	}

	c.tokens[resource] = tok
	return tok, nil
}
