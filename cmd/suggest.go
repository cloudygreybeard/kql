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
	suggestInputFile string
	suggestVerbose   bool
	suggestTimeout   int
	suggestFocus     string
)

var suggestCmd = &cobra.Command{
	Use:   "suggest [query]",
	Short: "Get AI-powered optimization suggestions for a KQL query",
	Long: `Analyze a KQL query and get suggestions for improvements.

The query can be provided as an argument, from a file (-f), or via stdin.

Suggestion focus areas (--focus):
  - performance:  Query execution speed and efficiency
  - readability:  Code clarity and maintainability
  - correctness:  Potential bugs or logic issues
  - all:          All of the above (default)

Uses the same AI providers as 'kql explain'.`,
	Example: `  # Get all suggestions
  kql suggest "T | where A > 0 | where B > 0 | project A, B"

  # Focus on performance
  kql suggest --focus performance "T | join kind=inner T2 on Id"

  # From file
  kql suggest -f query.kql

  # Use specific provider
  kql suggest --provider vertex --model gemini-1.5-pro "T | take 10"`,
	RunE: runSuggest,
}

func init() {
	rootCmd.AddCommand(suggestCmd)

	// Provider selection (reuse from explain)
	suggestCmd.Flags().StringVar(&aiProvider, "provider", "", "AI provider (ollama, instructlab, vertex, azure)")
	suggestCmd.Flags().StringVar(&aiModel, "model", "", "Model name")
	suggestCmd.Flags().Float32Var(&aiTemperature, "temperature", 0.3, "Temperature (0.0-1.0)")

	// Ollama
	suggestCmd.Flags().StringVar(&ollamaEndpoint, "ollama-endpoint", "", "Ollama endpoint URL")

	// Vertex AI
	suggestCmd.Flags().StringVar(&vertexProject, "vertex-project", "", "GCP project ID")
	suggestCmd.Flags().StringVar(&vertexLocation, "vertex-location", "", "GCP location")

	// Azure OpenAI
	suggestCmd.Flags().StringVar(&azureEndpoint, "azure-endpoint", "", "Azure OpenAI endpoint URL")
	suggestCmd.Flags().StringVar(&azureDeployment, "azure-deployment", "", "Azure OpenAI deployment name")

	// InstructLab
	suggestCmd.Flags().StringVar(&instructEndpoint, "instructlab-endpoint", "", "InstructLab endpoint URL")

	// Command options
	suggestCmd.Flags().StringVarP(&suggestInputFile, "file", "f", "", "Read query from file")
	suggestCmd.Flags().BoolVarP(&suggestVerbose, "verbose", "v", false, "Show additional context")
	suggestCmd.Flags().IntVar(&suggestTimeout, "timeout", 60, "Timeout in seconds")
	suggestCmd.Flags().StringVar(&suggestFocus, "focus", "all", "Suggestion focus: performance, readability, correctness, all")
}

func runSuggest(cmd *cobra.Command, args []string) error {
	// Get query input
	query, err := getInputFrom(args, suggestInputFile, os.Stdin, isTerminal)
	if err != nil {
		return err
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

	// Parse the query for context
	parseContext := getParseContextForSuggest(query)

	// Build prompt
	prompt := buildSuggestPrompt(query, parseContext, suggestFocus)

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(suggestTimeout)*time.Second)
	defer cancel()

	// Show progress
	if suggestVerbose {
		fmt.Fprintf(os.Stderr, "Using %s provider with model %s...\n", provider.Name(), provider.Model())
		fmt.Fprintf(os.Stderr, "Focus: %s\n", suggestFocus)
	}

	// Get suggestions
	suggestions, err := provider.Complete(ctx, prompt)
	if err != nil {
		return fmt.Errorf("getting suggestions: %w", err)
	}

	fmt.Println(suggestions)
	return nil
}

func getParseContextForSuggest(query string) string {
	result := kqlparser.Parse("input", query)

	var context strings.Builder
	context.WriteString("Query analysis:\n")

	if len(result.Errors) > 0 {
		context.WriteString(fmt.Sprintf("- Syntax errors: %d\n", len(result.Errors)))
		for _, err := range result.Errors {
			context.WriteString(fmt.Sprintf("  - %v\n", err))
		}
	} else {
		context.WriteString("- Syntax: valid\n")
	}

	// Count operators in the query (simple heuristic)
	operators := countOperators(query)
	if len(operators) > 0 {
		context.WriteString("- Operators used: ")
		context.WriteString(strings.Join(operators, ", "))
		context.WriteString("\n")
	}

	return context.String()
}

func countOperators(query string) []string {
	// Simple operator detection
	knownOps := []string{
		"where", "project", "extend", "summarize", "join", "union",
		"take", "top", "sort", "order", "distinct", "count", "limit",
		"mv-expand", "mv-apply", "parse", "evaluate", "render",
		"make-series", "lookup", "fork", "facet", "find", "search",
	}

	queryLower := strings.ToLower(query)
	var found []string

	for _, op := range knownOps {
		if strings.Contains(queryLower, "| "+op) || strings.Contains(queryLower, "|"+op) {
			found = append(found, op)
		}
	}

	return found
}

func buildSuggestPrompt(query, parseContext, focus string) string {
	var focusInstructions string

	switch focus {
	case "performance":
		focusInstructions = `Focus specifically on PERFORMANCE optimizations:
- Query execution efficiency
- Reducing data scanned (filter early)
- Join strategies and hints
- Aggregation optimizations
- Index usage
- Avoiding expensive operations`

	case "readability":
		focusInstructions = `Focus specifically on READABILITY improvements:
- Code clarity and structure
- Naming conventions
- Comments where helpful
- Breaking complex queries into steps
- Using let statements for reusability`

	case "correctness":
		focusInstructions = `Focus specifically on CORRECTNESS issues:
- Potential logic errors
- Edge cases not handled
- Type mismatches
- Null handling
- Time zone considerations
- Off-by-one errors in ranges`

	default: // "all"
		focusInstructions = `Analyze the query for:
1. PERFORMANCE - efficiency and speed improvements
2. READABILITY - clarity and maintainability
3. CORRECTNESS - potential bugs or logic issues`
	}

	return fmt.Sprintf(`You are a Kusto Query Language (KQL) expert. Analyze the following query and provide specific, actionable suggestions for improvement.

%s

For each suggestion:
1. Explain the issue or opportunity
2. Show the specific change (before → after)
3. Explain the benefit

If the query is already well-optimized, say so and explain why.

%s

Query:
%s`, focusInstructions, parseContext, "```kql\n"+query+"\n```")
}
