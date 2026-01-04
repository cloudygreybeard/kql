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
	"io"
	"os"
	"strings"

	"github.com/cloudygreybeard/kql/pkg/link"
	"github.com/spf13/cobra"
)

var (
	buildCluster  string
	buildDatabase string
	buildBaseURL  string
	buildFile     string
)

var linkBuildCmd = &cobra.Command{
	Use:   "build [QUERY]",
	Short: "Build a deep link from a KQL query",
	Long: `Build a compressed, shareable deep link URL from a KQL query
that opens directly in Azure Data Explorer.

The query can be provided via:
  - Positional argument (for short queries)
  - File (-f/--file flag)
  - Standard input (pipe or redirect)`,
	Example: `  # From stdin
  echo 'StormEvents | take 10' | kql link build -c help -d Samples

  # From file
  kql link build -c mycluster.westeurope -d mydb -f query.kql

  # As argument (for short queries)
  kql link build -c help -d Samples "print 'hello'"

  # Multi-line query via heredoc
  kql link build -c help -d Samples << 'EOF'
  StormEvents
  | where StartTime > ago(7d)
  | summarize count() by State
  | top 10 by count_
  EOF`,
	RunE: runLinkBuild,
}

func init() {
	linkCmd.AddCommand(linkBuildCmd)

	linkBuildCmd.Flags().StringVarP(&buildCluster, "cluster", "c", "", "Kusto cluster name (required)")
	linkBuildCmd.Flags().StringVarP(&buildDatabase, "database", "d", "", "Database name (required)")
	linkBuildCmd.Flags().StringVarP(&buildBaseURL, "base-url", "b", link.DefaultBaseURL, "Base URL for deep links")
	linkBuildCmd.Flags().StringVarP(&buildFile, "file", "f", "", "Read query from file")

	_ = linkBuildCmd.MarkFlagRequired("cluster")
	_ = linkBuildCmd.MarkFlagRequired("database")
}

func runLinkBuild(cmd *cobra.Command, args []string) error {
	query, err := getInput(args, buildFile)
	if err != nil {
		return err
	}

	result, err := link.Build(query, buildCluster, buildDatabase, buildBaseURL)
	if err != nil {
		return fmt.Errorf("build failed: %w", err)
	}

	fmt.Println(result)
	return nil
}

// getInput reads input from positional args, file, or stdin (in that priority order).
func getInput(args []string, filePath string) (string, error) {
	return getInputFrom(args, filePath, os.Stdin, isTerminal)
}

// isTerminal checks if the given file is a terminal
func isTerminal(f *os.File) bool {
	stat, _ := f.Stat()
	return (stat.Mode() & os.ModeCharDevice) != 0
}

// getInputFrom is the testable version of getInput
func getInputFrom(args []string, filePath string, stdin io.Reader, isTerminalFunc func(*os.File) bool) (string, error) {
	// Priority 1: positional argument
	if len(args) > 0 {
		return strings.TrimSpace(strings.Join(args, " ")), nil
	}

	// Priority 2: file
	if filePath != "" {
		data, err := os.ReadFile(filePath)
		if err != nil {
			return "", fmt.Errorf("reading file: %w", err)
		}
		result := strings.TrimSpace(string(data))
		if result == "" {
			return "", fmt.Errorf("file is empty: %s", filePath)
		}
		return result, nil
	}

	// Priority 3: stdin (only if not a terminal)
	if f, ok := stdin.(*os.File); ok {
		if isTerminalFunc(f) {
			return "", fmt.Errorf("no input provided (use -f <file>, stdin, or pass query as argument)")
		}
	}

	data, err := io.ReadAll(stdin)
	if err != nil {
		return "", fmt.Errorf("reading stdin: %w", err)
	}

	result := strings.TrimSpace(string(data))
	if result == "" {
		return "", fmt.Errorf("empty input from stdin")
	}

	return result, nil
}
