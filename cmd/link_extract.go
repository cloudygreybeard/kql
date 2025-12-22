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
	"fmt"

	"github.com/cloudygreybeard/kql/pkg/link"
	"github.com/spf13/cobra"
)

var extractFile string

var linkExtractCmd = &cobra.Command{
	Use:   "extract [URL]",
	Short: "Extract the query from a deep link",
	Long: `Extract the original KQL query from a Kusto deep link URL.

The URL can be provided via:
  - Positional argument
  - File (-f/--file flag)
  - Standard input (pipe or redirect)`,
	Example: `  # As argument
  kql link extract "https://dataexplorer.azure.com/clusters/help/databases/Samples?query=..."

  # From stdin
  echo 'https://dataexplorer.azure.com/...' | kql link extract

  # From file
  kql link extract -f url.txt`,
	RunE: runLinkExtract,
}

func init() {
	linkCmd.AddCommand(linkExtractCmd)

	linkExtractCmd.Flags().StringVarP(&extractFile, "file", "f", "", "Read URL from file")
}

func runLinkExtract(cmd *cobra.Command, args []string) error {
	input, err := getInput(args, extractFile)
	if err != nil {
		return err
	}

	query, err := link.Extract(input)
	if err != nil {
		return fmt.Errorf("extract failed: %w", err)
	}

	fmt.Println(query)
	return nil
}

