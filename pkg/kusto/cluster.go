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

// Package kusto executes KQL queries against Azure Data Explorer clusters and
// renders their results.
package kusto

import (
	"fmt"
	"net/url"
	"strings"
)

// DefaultDomain is appended to a bare cluster name to form a host name.
const DefaultDomain = "kusto.windows.net"

// knownDomains are host suffixes recognised as complete Kusto host names, so
// that a reference carrying one is not further expanded. A reference such as
// "mycluster.westeurope" contains a dot yet is still a short name, so the presence
// of a dot alone cannot distinguish the two forms.
var knownDomains = []string{
	".kusto.windows.net",
	".kustomfa.windows.net",
	".kusto.chinacloudapi.cn",
	".kusto.usgovcloudapi.net",
	".kusto.core.eaglex.ic.gov",
	".kusto.core.microsoft.scloud",
	".kusto.fabric.microsoft.com",
	".playfab.com",
}

// ResolveCluster expands a cluster reference into a data-plane base URL.
//
// It accepts, in order of precedence, an alias defined in aliases, an absolute
// URL, a complete host name, and a short name optionally qualified by region.
// Short names are expanded with [DefaultDomain]:
//
//	help                -> https://help.kusto.windows.net
//	mycluster.westeurope    -> https://mycluster.westeurope.kusto.windows.net
//	adx.example.net     -> https://adx.example.net
//	https://host:8080   -> https://host:8080
//
// Aliases are resolved once, so an alias may name any other accepted form but
// may not name a further alias.
func ResolveCluster(ref string, aliases map[string]string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", fmt.Errorf("cluster must not be empty")
	}

	if target, ok := aliases[ref]; ok {
		resolved := strings.TrimSpace(target)
		if resolved == "" {
			return "", fmt.Errorf("cluster alias %q resolves to an empty value", ref)
		}
		ref = resolved
	}

	if hasScheme(ref) {
		return normaliseURL(ref)
	}
	return normaliseURL("https://" + expandHost(ref))
}

// hasScheme reports whether ref already carries a URL scheme.
func hasScheme(ref string) bool {
	return strings.Contains(ref, "://")
}

// expandHost appends [DefaultDomain] to a short cluster name.
func expandHost(ref string) string {
	lower := strings.ToLower(ref)
	// A port or path implies a host the caller has spelled out in full.
	if strings.ContainsAny(ref, ":/") {
		return ref
	}
	for _, domain := range knownDomains {
		if strings.HasSuffix(lower, domain) {
			return ref
		}
	}
	return ref + "." + DefaultDomain
}

// normaliseURL validates a cluster URL and strips any trailing path, leaving a
// base URL that request paths can be appended to.
func normaliseURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid cluster %q: %w", raw, err)
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return "", fmt.Errorf("invalid cluster %q: scheme must be http or https", raw)
	}
	if u.Host == "" {
		return "", fmt.Errorf("invalid cluster %q: no host", raw)
	}
	return u.Scheme + "://" + u.Host, nil
}

// Resource returns the Microsoft Entra ID audience for a cluster base URL.
//
// Azure Data Explorer accepts a token whose audience is the cluster itself,
// which is narrower than the shared "https://kusto.kusto.windows.net" audience
// and is therefore preferred.
func Resource(endpoint string) string {
	return strings.TrimSuffix(endpoint, "/")
}
