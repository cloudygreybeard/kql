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
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/cloudygreybeard/kqlparser"
	"github.com/spf13/cobra"
)

var lintCmd = &cobra.Command{
	Use:   "lint [file...]",
	Short: "Validate KQL query syntax and semantics",
	Long: `Lint validates KQL queries for syntax errors and optionally
performs semantic analysis including type checking and name resolution.

If no files are provided, reads from stdin.
Use '-' as a filename to explicitly read from stdin.`,
	Example: `  # Lint from stdin
  echo "T | where x > 10" | kql lint

  # Lint a file
  kql lint query.kql

  # Lint with semantic checks
  kql lint --strict query.kql

  # Lint multiple files
  kql lint queries/*.kql

  # JSON output for CI
  kql lint --format json --strict query.kql`,
	RunE: runLint,
}

var (
	lintStrict bool
	lintQuiet  bool
	lintFormat string
)

func init() {
	rootCmd.AddCommand(lintCmd)

	lintCmd.Flags().BoolVar(&lintStrict, "strict", false, "Enable semantic analysis (type checking, name resolution)")
	lintCmd.Flags().BoolVar(&lintQuiet, "quiet", false, "Only output errors (no success messages)")
	lintCmd.Flags().StringVar(&lintFormat, "format", "text", "Output format: text, json")
}

// LintDiagnostic represents a single diagnostic message.
type LintDiagnostic struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Column   int    `json:"column"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

// osExit is a variable to allow testing
var osExit = os.Exit

func runLint(cmd *cobra.Command, args []string) error {
	hasErrors, err := doLint(args, os.Stdin)
	if err != nil {
		return err
	}
	if hasErrors {
		osExit(1)
	}
	return nil
}

// doLint performs the actual linting and returns whether errors were found.
// Separated from runLint to enable testing without os.Exit.
func doLint(args []string, stdin io.Reader) (bool, error) {
	var allDiagnostics []LintDiagnostic

	if len(args) == 0 {
		// Read from stdin
		diags, err := lintReader("stdin", stdin)
		if err != nil {
			return false, err
		}
		allDiagnostics = append(allDiagnostics, diags...)
	} else {
		for _, filename := range args {
			var diags []LintDiagnostic
			var err error

			if filename == "-" {
				diags, err = lintReader("stdin", stdin)
			} else {
				diags, err = lintFile(filename)
			}

			if err != nil {
				return false, err
			}
			allDiagnostics = append(allDiagnostics, diags...)
		}
	}

	// Check if any errors
	hasErrors := false
	for _, d := range allDiagnostics {
		if d.Severity == "error" {
			hasErrors = true
			break
		}
	}

	// Output results
	if err := outputDiagnostics(allDiagnostics, hasErrors); err != nil {
		return false, err
	}

	return hasErrors, nil
}

func lintFile(filename string) ([]LintDiagnostic, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("cannot open file %s: %w", filename, err)
	}
	defer f.Close()

	return lintReader(filename, f)
}

func lintReader(filename string, r io.Reader) ([]LintDiagnostic, error) {
	// Read all content
	var content strings.Builder
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		content.WriteString(scanner.Text())
		content.WriteString("\n")
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading %s: %w", filename, err)
	}

	return lintQuery(filename, content.String())
}

func lintQuery(filename, query string) ([]LintDiagnostic, error) {
	var diagnostics []LintDiagnostic

	if lintStrict {
		// Full semantic analysis
		result := kqlparser.ParseAndAnalyze(filename, query, nil)
		for _, diag := range result.Errors() {
			diagnostics = append(diagnostics, LintDiagnostic{
				File:     filename,
				Line:     diag.Pos.Line,
				Column:   diag.Pos.Column,
				Severity: "error",
				Message:  diag.Message,
			})
		}
		for _, diag := range result.Warnings() {
			diagnostics = append(diagnostics, LintDiagnostic{
				File:     filename,
				Line:     diag.Pos.Line,
				Column:   diag.Pos.Column,
				Severity: "warning",
				Message:  diag.Message,
			})
		}
	} else {
		// Syntax-only parsing
		result := kqlparser.Parse(filename, query)
		for _, err := range result.Errors {
			diag := parseErrorToDiagnostic(filename, err)
			diagnostics = append(diagnostics, diag)
		}
	}

	return diagnostics, nil
}

func outputDiagnostics(diagnostics []LintDiagnostic, hasErrors bool) error {
	switch lintFormat {
	case "json":
		return outputJSON(diagnostics)
	case "text":
		return outputText(diagnostics, hasErrors)
	default:
		return fmt.Errorf("unknown format: %s", lintFormat)
	}
}

func outputJSON(diagnostics []LintDiagnostic) error {
	for _, d := range diagnostics {
		data, err := json.Marshal(d)
		if err != nil {
			return err
		}
		fmt.Println(string(data))
	}
	return nil
}

func outputText(diagnostics []LintDiagnostic, hasErrors bool) error {
	for _, d := range diagnostics {
		fmt.Printf("%s:%d:%d: %s: %s\n", d.File, d.Line, d.Column, d.Severity, d.Message)
	}

	if !lintQuiet && len(diagnostics) == 0 {
		fmt.Println("No issues found.")
	}

	return nil
}

// parseErrorToDiagnostic extracts position info from a parse error.
// Parser errors are formatted as "file:line:col: message"
var errPosRegex = regexp.MustCompile(`^([^:]+):(\d+):(\d+): (.+)$`)

func parseErrorToDiagnostic(filename string, err error) LintDiagnostic {
	errStr := err.Error()
	matches := errPosRegex.FindStringSubmatch(errStr)
	if matches != nil {
		line, _ := strconv.Atoi(matches[2])
		col, _ := strconv.Atoi(matches[3])
		return LintDiagnostic{
			File:     filename,
			Line:     line,
			Column:   col,
			Severity: "error",
			Message:  matches[4],
		}
	}
	// Fallback if parsing fails
	return LintDiagnostic{
		File:     filename,
		Line:     1,
		Column:   1,
		Severity: "error",
		Message:  errStr,
	}
}
