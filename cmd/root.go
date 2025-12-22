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

package cmd

import (
	"github.com/spf13/cobra"
)

// Version information (set via ldflags at build time)
var (
	Version   = "dev"
	GitCommit = "unknown"
	BuildDate = "unknown"
)

var rootCmd = &cobra.Command{
	Use:   "kql",
	Short: "A CLI toolkit for Kusto Query Language (KQL)",
	Long: `kql is a command-line toolkit for working with Kusto Query Language (KQL)
and Azure Data Explorer.

Current capabilities:
  - Build shareable deep links from KQL queries
  - Extract queries from deep links

Based on the Microsoft Kusto deep link specification:
https://learn.microsoft.com/en-us/kusto/api/rest/deeplink`,
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

