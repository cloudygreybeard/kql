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
	"github.com/spf13/cobra"
)

var (
	generateInputFile string
	generateVerbose   bool
	generateTimeout   int
	generateTable     string
	generateSchema    string
)

var generateCmd = &cobra.Command{
	Use:   "generate [description]",
	Short: "Generate KQL from a natural language description",
	Long: `Generate a KQL query from a natural language description.

The description can be provided as an argument, from a file (-f), or via stdin.

Optionally provide table name and schema for more accurate generation.

Uses the same AI providers as 'kql explain'.`,
	Example: `  # Simple generation
  kql generate "count events by state"

  # With table context
  kql generate --table StormEvents "show top 10 states by damage"

  # With schema hint
  kql generate --table StormEvents --schema "State, StartTime, DamageProperty" \
      "find events in Texas with damage over 1 million"

  # From file
  echo "get hourly event counts for the last week" | kql generate --table Events

  # Use specific provider
  kql generate --provider vertex --model gemini-1.5-pro "summarize by category"`,
	RunE: runGenerate,
}

func init() {
	rootCmd.AddCommand(generateCmd)

	// Provider selection (reuse from explain)
	generateCmd.Flags().StringVar(&aiProvider, "provider", "", "AI provider (ollama, instructlab, vertex, azure)")
	generateCmd.Flags().StringVar(&aiModel, "model", "", "Model name")
	generateCmd.Flags().Float32Var(&aiTemperature, "temperature", 0.2, "Temperature (0.0-1.0)")

	// Ollama
	generateCmd.Flags().StringVar(&ollamaEndpoint, "ollama-endpoint", "", "Ollama endpoint URL")

	// Vertex AI
	generateCmd.Flags().StringVar(&vertexProject, "vertex-project", "", "GCP project ID")
	generateCmd.Flags().StringVar(&vertexLocation, "vertex-location", "", "GCP location")

	// Azure OpenAI
	generateCmd.Flags().StringVar(&azureEndpoint, "azure-endpoint", "", "Azure OpenAI endpoint URL")
	generateCmd.Flags().StringVar(&azureDeployment, "azure-deployment", "", "Azure OpenAI deployment name")

	// InstructLab
	generateCmd.Flags().StringVar(&instructEndpoint, "instructlab-endpoint", "", "InstructLab endpoint URL")

	// Command options
	generateCmd.Flags().StringVarP(&generateInputFile, "file", "f", "", "Read description from file")
	generateCmd.Flags().BoolVarP(&generateVerbose, "verbose", "v", false, "Show additional context")
	generateCmd.Flags().IntVar(&generateTimeout, "timeout", 60, "Timeout in seconds")

	// Context options
	generateCmd.Flags().StringVarP(&generateTable, "table", "t", "", "Target table name")
	generateCmd.Flags().StringVarP(&generateSchema, "schema", "s", "", "Table schema (comma-separated columns)")
}

func runGenerate(cmd *cobra.Command, args []string) error {
	// Get description input
	description, err := getInputFrom(args, generateInputFile, os.Stdin, isTerminal)
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

	// Build prompt
	prompt := buildGeneratePrompt(description, generateTable, generateSchema)

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(generateTimeout)*time.Second)
	defer cancel()

	// Show progress
	if generateVerbose {
		fmt.Fprintf(os.Stderr, "Using %s provider with model %s...\n", provider.Name(), provider.Model())
		if generateTable != "" {
			fmt.Fprintf(os.Stderr, "Target table: %s\n", generateTable)
		}
	}

	// Generate query
	result, err := provider.Complete(ctx, prompt)
	if err != nil {
		return fmt.Errorf("generating query: %w", err)
	}

	// Extract just the KQL from the response
	query := extractKQL(result)
	fmt.Println(query)

	return nil
}

func buildGeneratePrompt(description, table, schema string) string {
	var context strings.Builder

	context.WriteString(`You are a Kusto Query Language (KQL) expert. Generate a KQL query based on the user's natural language description.

Rules:
1. Output ONLY the KQL query, no explanations
2. Use proper KQL syntax and operators
3. Include comments only if the query is complex
4. Prefer efficient query patterns
`)

	if table != "" {
		context.WriteString(fmt.Sprintf("\nTarget table: %s\n", table))
	}

	if schema != "" {
		context.WriteString(fmt.Sprintf("Available columns: %s\n", schema))
	}

	context.WriteString(fmt.Sprintf("\nDescription: %s\n", description))
	context.WriteString("\nGenerate the KQL query:")

	return context.String()
}

// extractKQL attempts to extract just the KQL code from an LLM response.
// Handles responses that include markdown code blocks or explanatory text.
func extractKQL(response string) string {
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

	// If no code blocks, try to find lines that look like KQL
	// (start with a table name or common operators)
	lines := strings.Split(response, "\n")
	var kqlLines []string
	inQuery := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Skip empty lines at the start
		if !inQuery && trimmed == "" {
			continue
		}

		// Detect start of KQL (table name or let statement)
		if !inQuery {
			if looksLikeKQLStart(trimmed) {
				inQuery = true
			}
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

func looksLikeKQLStart(line string) bool {
	lower := strings.ToLower(line)

	// Common KQL starting patterns
	starters := []string{
		"let ", "//", "/*",
	}

	for _, s := range starters {
		if strings.HasPrefix(lower, s) {
			return true
		}
	}

	// Starts with what looks like a table name (alphanumeric, no spaces before pipe)
	if len(line) > 0 {
		// Table names typically start with a letter
		if (line[0] >= 'A' && line[0] <= 'Z') || (line[0] >= 'a' && line[0] <= 'z') {
			return true
		}
	}

	return false
}

func looksLikeExplanation(line string) bool {
	lower := strings.ToLower(line)

	// Common explanation starters
	explanations := []string{
		"this query", "the query", "this will", "explanation:",
		"note:", "here's", "here is", "the above",
	}

	for _, e := range explanations {
		if strings.HasPrefix(lower, e) {
			return true
		}
	}

	return false
}

