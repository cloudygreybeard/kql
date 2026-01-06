// Copyright 2026 cloudygreybeard
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/cloudygreybeard/kql/pkg/ai"
	"github.com/cloudygreybeard/kqlparser"
	"github.com/spf13/cobra"
)

var (
	fixInputFile string
	fixVerbose   bool
	fixTimeout   int
	fixDryRun    bool

	// Validation flags for fix
	fixRetries int
	fixStrict  bool
)

var fixCmd = &cobra.Command{
	Use:   "fix [query]",
	Short: "Get AI-suggested fixes for KQL syntax errors",
	Long: `Analyze a KQL query with syntax errors and get AI-suggested fixes.

The query is first parsed to identify errors, then AI suggests corrections.
The query can be provided as an argument, from a file (-f), or via stdin.

Use --dry-run to see the suggested fix without outputting it.
Use --verbose to see the original errors and AI reasoning.

Uses the same AI providers as 'kql explain'.`,
	Example: `  # Fix a query with syntax errors
  kql fix "StormEvents | where State = 'TEXAS'"

  # Fix from file
  kql fix -f broken_query.kql

  # Dry run (show analysis without outputting fixed query)
  kql fix --dry-run "T | summarize count( by State"

  # Verbose mode (show errors and reasoning)
  kql fix -v "T | where x >"`,
	RunE: runFix,
}

func init() {
	rootCmd.AddCommand(fixCmd)

	// Provider selection (reuse from explain)
	fixCmd.Flags().StringVar(&aiProvider, "provider", "", "AI provider (ollama, instructlab, vertex, azure)")
	fixCmd.Flags().StringVar(&aiModel, "model", "", "Model name")
	fixCmd.Flags().Float32Var(&aiTemperature, "temperature", 0.1, "Temperature (0.0-1.0)")

	// Ollama
	fixCmd.Flags().StringVar(&ollamaEndpoint, "ollama-endpoint", "", "Ollama endpoint URL")

	// Vertex AI
	fixCmd.Flags().StringVar(&vertexProject, "vertex-project", "", "GCP project ID")
	fixCmd.Flags().StringVar(&vertexLocation, "vertex-location", "", "GCP location")

	// Azure OpenAI
	fixCmd.Flags().StringVar(&azureEndpoint, "azure-endpoint", "", "Azure OpenAI endpoint URL")
	fixCmd.Flags().StringVar(&azureDeployment, "azure-deployment", "", "Azure OpenAI deployment name")

	// InstructLab
	fixCmd.Flags().StringVar(&instructEndpoint, "instructlab-endpoint", "", "InstructLab endpoint URL")

	// Command options
	fixCmd.Flags().StringVarP(&fixInputFile, "file", "f", "", "Read query from file")
	fixCmd.Flags().BoolVarP(&fixVerbose, "verbose", "v", false, "Show errors and reasoning")
	fixCmd.Flags().IntVar(&fixTimeout, "timeout", 60, "Timeout in seconds")
	fixCmd.Flags().BoolVar(&fixDryRun, "dry-run", false, "Show analysis without outputting fixed query")

	// Retry and validation options
	fixCmd.Flags().IntVar(&fixRetries, "retries", 2, "Number of retries if fix still has errors")
	fixCmd.Flags().BoolVar(&fixStrict, "strict", false, "Fail with exit code 1 if fix still has errors")
}

func runFix(cmd *cobra.Command, args []string) error {
	// Get query input
	query, err := getInputFrom(args, fixInputFile, os.Stdin, isTerminal)
	if err != nil {
		return err
	}

	// Parse the query to find errors
	result := kqlparser.Parse("input", query)

	if len(result.Errors) == 0 {
		if fixVerbose {
			fmt.Fprintln(os.Stderr, "No syntax errors found in query.")
		}
		// Output the original query if no errors
		fmt.Println(query)
		return nil
	}

	if fixVerbose {
		fmt.Fprintln(os.Stderr, "Found errors:")
		for _, e := range result.Errors {
			fmt.Fprintf(os.Stderr, "  - %v\n", e)
		}
		fmt.Fprintln(os.Stderr)
	}

	// Build AI config
	cfg := buildAIConfig()

	// Load file config and merge
	fileCfg, err := ai.LoadConfigFile()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: error loading config file: %v\n", err)
	}
	cfg = ai.MergeFileConfig(cfg, fileCfg)

	// Apply defaults if still empty
	if cfg.Provider == "" {
		cfg.Provider = "ollama"
	}

	// Create provider
	provider, err := ai.NewProvider(cfg)
	if err != nil {
		return fmt.Errorf("creating AI provider: %w", err)
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(fixTimeout)*time.Second)
	defer cancel()

	// Show progress
	if fixVerbose {
		fmt.Fprintf(os.Stderr, "Using %s provider with model %s...\n", provider.Name(), provider.Model())
	}

	// Retry loop for fixing
	maxAttempts := fixRetries + 1
	var fixedQuery string
	var fixErrors []error
	currentQuery := query
	currentErrors := result.Errors

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if fixVerbose {
			fmt.Fprintf(os.Stderr, "Attempt %d/%d: requesting fix...\n", attempt, maxAttempts)
		}

		// Build prompt with current errors
		errorContext := buildErrorContext(currentQuery, currentErrors)
		prompt := buildFixPrompt(currentQuery, errorContext)

		// Get fix suggestion
		response, err := provider.Complete(ctx, prompt)
		if err != nil {
			return fmt.Errorf("getting fix suggestion (attempt %d): %w", attempt, err)
		}

		// Extract the fixed query
		fixedQuery = extractFixedQuery(response)

		// Validate the fix
		fixResult := kqlparser.Parse("fixed", fixedQuery)
		if len(fixResult.Errors) == 0 {
			if fixVerbose {
				fmt.Fprintln(os.Stderr, "  ✓ Fix is syntactically valid")
			}
			fixErrors = nil
			break
		}

		fixErrors = fixResult.Errors
		if fixVerbose {
			fmt.Fprintf(os.Stderr, "  ✗ Fix still has %d error(s)\n", len(fixErrors))
			for _, e := range fixErrors {
				fmt.Fprintf(os.Stderr, "    - %v\n", e)
			}
		}

		// For next attempt, use the AI's fix as the starting point
		currentQuery = fixedQuery
		currentErrors = fixErrors
	}

	if fixDryRun {
		fmt.Fprintln(os.Stderr, "=== Original Query ===")
		fmt.Fprintln(os.Stderr, query)
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "=== Suggested Fix ===")
		fmt.Fprintln(os.Stderr, fixedQuery)
		fmt.Fprintln(os.Stderr)

		if len(fixErrors) == 0 {
			fmt.Fprintln(os.Stderr, "✓ Suggested fix is syntactically valid")
		} else {
			fmt.Fprintln(os.Stderr, "⚠ Suggested fix still has errors:")
			for _, e := range fixErrors {
				fmt.Fprintf(os.Stderr, "  - %v\n", e)
			}
		}
		return nil
	}

	// Handle result based on validation outcome
	if len(fixErrors) > 0 {
		if fixStrict {
			fmt.Fprintf(os.Stderr, "Error: failed to generate valid fix after %d attempt(s)\n", maxAttempts)
			for _, e := range fixErrors {
				fmt.Fprintf(os.Stderr, "  - %v\n", e)
			}
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "⚠ Warning: fix still has syntax errors (after %d attempt(s))\n", maxAttempts)
	}

	// Output the fixed query
	fmt.Println(fixedQuery)
	return nil
}

func buildErrorContext(query string, errors []error) string {
	var sb strings.Builder

	sb.WriteString("Errors found:\n")
	for i, e := range errors {
		sb.WriteString(fmt.Sprintf("%d. %v\n", i+1, e))
	}

	return sb.String()
}

func buildFixPrompt(query, errorContext string) string {
	return fmt.Sprintf(`You are a Kusto Query Language (KQL) expert. Fix the syntax errors in the following query.

Rules:
1. Output ONLY the corrected KQL query
2. Preserve the original intent of the query
3. Make minimal changes to fix the errors
4. Do not add features or optimizations, only fix errors

%s

Original query with errors:
%s

Output the corrected query:`, errorContext, "```kql\n"+query+"\n```")
}

// extractFixedQuery extracts the fixed query from the LLM response.
func extractFixedQuery(response string) string {
	response = strings.TrimSpace(response)

	// Check for markdown code blocks
	if strings.Contains(response, "```") {
		// Try to find kql/kusto code block first
		for _, lang := range []string{"```kql", "```kusto", "```"} {
			if idx := strings.Index(response, lang); idx != -1 {
				start := idx + len(lang)
				// Find the closing ```
				if end := strings.Index(response[start:], "```"); end != -1 {
					extracted := strings.TrimSpace(response[start : start+end])
					if extracted != "" {
						return extracted
					}
				}
			}
		}
	}

	// Try to extract lines that look like KQL
	lines := strings.Split(response, "\n")
	var kqlLines []string
	inQuery := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Skip empty lines at the start
		if !inQuery && trimmed == "" {
			continue
		}

		// Detect start of KQL
		if !inQuery && looksLikeKQLStart(trimmed) {
			inQuery = true
		}

		if inQuery {
			// Stop at explanatory text
			if looksLikeExplanation(trimmed) {
				break
			}
			kqlLines = append(kqlLines, line)
		}
	}

	if len(kqlLines) > 0 {
		return strings.TrimSpace(strings.Join(kqlLines, "\n"))
	}

	// Fallback: return as-is
	return response
}
