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
	"fmt"
	"strings"
)

// Source names where bearer tokens are obtained from.
type Source int

const (
	// SourceAuto consults each provider in turn, preferring an explicitly
	// supplied token, then a configured helper, then the Azure CLI.
	SourceAuto Source = iota
	// SourceAzureCLI uses only the Azure CLI.
	SourceAzureCLI
	// SourceExec uses only the configured credential helper.
	SourceExec
	// SourceEnv uses only the token held in the environment.
	SourceEnv
)

// String returns the flag spelling of s.
func (s Source) String() string {
	switch s {
	case SourceAzureCLI:
		return "az"
	case SourceExec:
		return "exec"
	case SourceEnv:
		return "env"
	default:
		return "auto"
	}
}

// sources lists the accepted spellings, in the order they are documented.
var sources = []string{"auto", "az", "exec", "env"}

// ParseSource converts a flag or configuration value into a Source.
func ParseSource(s string) (Source, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "auto":
		return SourceAuto, nil
	case "az", "azure-cli":
		return SourceAzureCLI, nil
	case "exec", "command":
		return SourceExec, nil
	case "env":
		return SourceEnv, nil
	default:
		return SourceAuto, fmt.Errorf("unknown auth source %q: want one of %s",
			s, strings.Join(sources, ", "))
	}
}
