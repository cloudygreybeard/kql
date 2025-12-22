// Copyright 2024 cloudygreybeard
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

// Package link provides functions for building and extracting Kusto deep links.
//
// Based on the Microsoft Kusto deep link specification:
// https://learn.microsoft.com/en-us/kusto/api/rest/deeplink
package link

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"fmt"
	"io"
	"net/url"
	"strings"
)

// DefaultBaseURL is the Azure Data Explorer web interface URL.
const DefaultBaseURL = "https://dataexplorer.azure.com"

// Build creates a Kusto deep link URL from the given KQL query.
//
// The query is compressed with gzip and encoded with base64 to create
// shorter URLs that fit within browser URI length limits (~2000 chars).
//
// Parameters:
//   - query: The KQL query text
//   - cluster: The Kusto cluster name (e.g., "mycluster.westeurope")
//   - database: The database name
//   - baseURL: The base URL for the deep link (defaults to DefaultBaseURL if empty)
//
// Returns the complete deep link URL.
func Build(query, cluster, database, baseURL string) (string, error) {
	if query == "" {
		return "", fmt.Errorf("query cannot be empty")
	}
	if cluster == "" {
		return "", fmt.Errorf("cluster cannot be empty")
	}
	if database == "" {
		return "", fmt.Errorf("database cannot be empty")
	}
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}

	// Compress with gzip
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write([]byte(query)); err != nil {
		return "", fmt.Errorf("compress query: %w", err)
	}
	if err := gz.Close(); err != nil {
		return "", fmt.Errorf("finalize compression: %w", err)
	}

	// Encode with base64, then URL-encode
	encoded := base64.StdEncoding.EncodeToString(buf.Bytes())
	encodedQuery := url.QueryEscape(encoded)

	// Build the URL
	return fmt.Sprintf("%s/clusters/%s/databases/%s?query=%s",
		strings.TrimSuffix(baseURL, "/"),
		url.PathEscape(cluster),
		url.PathEscape(database),
		encodedQuery,
	), nil
}

// Extract retrieves the original KQL query from a Kusto deep link URL.
//
// This is the reverse operation of Build - it parses the URL, extracts
// the query parameter, and decompresses it.
func Extract(link string) (string, error) {
	parsedURL, err := url.Parse(link)
	if err != nil {
		return "", fmt.Errorf("parse URL: %w", err)
	}

	// Query().Get() already URL-decodes the value
	encodedQuery := parsedURL.Query().Get("query")
	if encodedQuery == "" {
		return "", fmt.Errorf("no 'query' parameter found in URL")
	}

	// Base64 decode
	compressed, err := base64.StdEncoding.DecodeString(encodedQuery)
	if err != nil {
		return "", fmt.Errorf("base64 decode: %w", err)
	}

	// Gzip decompress
	gz, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return "", fmt.Errorf("initialize decompression: %w", err)
	}
	defer gz.Close()

	query, err := io.ReadAll(gz)
	if err != nil {
		return "", fmt.Errorf("decompress query: %w", err)
	}

	return string(query), nil
}

