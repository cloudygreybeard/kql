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

var linkCmd = &cobra.Command{
	Use:   "link",
	Short: "Work with Kusto deep links",
	Long: `Commands for building KQL queries into shareable deep links
and extracting queries from existing links.

Deep links open directly in Azure Data Explorer with the query pre-filled,
making them ideal for documentation, runbooks, and issue trackers.`,
}

func init() {
	rootCmd.AddCommand(linkCmd)
}


