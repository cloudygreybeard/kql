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
	generateDebug     bool
	generateTimeout   int
	generateTable     string
	generateSchema    string

	// Validation flags
	generateNoValidate         bool
	generateStrict             bool
	generateRetries            int
	generateNoFeedback         bool
	generateNoFeedbackErrors   bool
	generateNoFeedbackHints    bool
	generateNoFeedbackExamples bool
	generateNoFeedbackProg     bool
	generateNoTempAdjust       bool
	generateTempIncrement      float32
	generateTempMax            float32
	generatePreset             string
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
	generateCmd.Flags().BoolVar(&generateDebug, "debug", false, "Show raw LLM responses (for troubleshooting)")
	generateCmd.Flags().IntVar(&generateTimeout, "timeout", 60, "Timeout in seconds")

	// Context options
	generateCmd.Flags().StringVarP(&generateTable, "table", "t", "", "Target table name")
	generateCmd.Flags().StringVarP(&generateSchema, "schema", "s", "", "Table schema (comma-separated columns)")

	// Validation flags
	generateCmd.Flags().BoolVar(&generateNoValidate, "no-validate", false, "Disable validation")
	generateCmd.Flags().BoolVar(&generateStrict, "strict", false, "Fail with exit code 1 if validation fails")
	generateCmd.Flags().IntVar(&generateRetries, "retries", 2, "Number of retry attempts on validation failure")

	// Feedback control flags
	generateCmd.Flags().BoolVar(&generateNoFeedback, "no-feedback", false, "Disable all feedback strategies")
	generateCmd.Flags().BoolVar(&generateNoFeedbackErrors, "no-feedback-errors", false, "Disable error feedback")
	generateCmd.Flags().BoolVar(&generateNoFeedbackHints, "no-feedback-hints", false, "Disable hints")
	generateCmd.Flags().BoolVar(&generateNoFeedbackExamples, "no-feedback-examples", false, "Disable examples")
	generateCmd.Flags().BoolVar(&generateNoFeedbackProg, "no-feedback-progressive", false, "Disable progressive detail")

	// Temperature adjustment flags
	generateCmd.Flags().BoolVar(&generateNoTempAdjust, "no-retry-temp-adjust", false, "Disable temperature adjustment on retry")
	generateCmd.Flags().Float32Var(&generateTempIncrement, "retry-temp-increment", 0, "Temperature increment per retry")
	generateCmd.Flags().Float32Var(&generateTempMax, "retry-temp-max", 0, "Max temperature on retry")

	// Presets
	generateCmd.Flags().StringVar(&generatePreset, "preset", "", "Preset: minimal, balanced, thorough, strict")
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

	// Apply validation config from flags and environment
	valCfg := buildValidationConfig(cfg.Validation)

	// Create provider
	provider, err := ai.NewProvider(cfg)
	if err != nil {
		return fmt.Errorf("creating AI provider: %w", err)
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(generateTimeout)*time.Second)
	defer cancel()

	// Show progress
	if generateVerbose {
		fmt.Fprintf(os.Stderr, "Using %s provider with model %s...\n", provider.Name(), provider.Model())
		if generateTable != "" {
			fmt.Fprintf(os.Stderr, "Target table: %s\n", generateTable)
		}
		if valCfg.Enabled {
			fmt.Fprintf(os.Stderr, "Validation: enabled (retries=%d, strict=%v)\n", valCfg.Retries, valCfg.Strict)
		} else {
			fmt.Fprintf(os.Stderr, "Validation: disabled\n")
		}
	}

	// Build request
	req := ai.GenerateRequest{
		Prompt: description,
		Table:  generateTable,
		Schema: generateSchema,
	}

	// Verbose and debug output writers
	var verboseWriter, debugWriter *os.File
	if generateVerbose {
		verboseWriter = os.Stderr
	}
	if generateDebug {
		debugWriter = os.Stderr
	}

	// Generate with validation
	result, err := ai.GenerateWithValidation(
		ctx,
		provider,
		req,
		valCfg,
		cfg.Temperature,
		func(r ai.GenerateRequest) string {
			return buildGeneratePrompt(r.Prompt, r.Table, r.Schema)
		},
		extractKQL,
		verboseWriter,
		debugWriter,
	)
	if err != nil {
		return err
	}

	// Handle result based on validation outcome
	if !result.Valid {
		if valCfg.Strict {
			fmt.Fprint(os.Stderr, ai.FormatValidationError(result))
			os.Exit(1)
		}
		fmt.Fprint(os.Stderr, ai.FormatValidationWarning(result))
	}

	fmt.Println(result.Query)
	return nil
}

// buildValidationConfig builds validation config from flags, environment, and defaults.
func buildValidationConfig(base ai.ValidationConfig) ai.ValidationConfig {
	cfg := base

	// Apply preset first
	switch generatePreset {
	case "minimal":
		cfg.Retries = 0
		cfg.Feedback.Hints = false
		cfg.Feedback.Examples = false
	case "balanced":
		// Use defaults
	case "thorough":
		cfg.Retries = 5
		cfg.Feedback.Progressive = true
	case "strict":
		cfg.Strict = true
		cfg.Retries = 3
	}

	// Override with explicit flags
	if generateNoValidate {
		cfg.Enabled = false
	}
	if generateStrict {
		cfg.Strict = true
	}
	// Always apply retries flag (default is 2, which is also the config default)
	cfg.Retries = generateRetries

	// Feedback flags
	if generateNoFeedback {
		cfg.Feedback.Errors = false
		cfg.Feedback.Hints = false
		cfg.Feedback.Examples = false
		cfg.Feedback.Progressive = false
	} else {
		if generateNoFeedbackErrors {
			cfg.Feedback.Errors = false
		}
		if generateNoFeedbackHints {
			cfg.Feedback.Hints = false
		}
		if generateNoFeedbackExamples {
			cfg.Feedback.Examples = false
		}
		if generateNoFeedbackProg {
			cfg.Feedback.Progressive = false
		}
	}

	// Temperature adjustment flags
	if generateNoTempAdjust {
		cfg.Temp.Adjust = false
	}
	if generateTempIncrement > 0 {
		cfg.Temp.Increment = generateTempIncrement
	}
	if generateTempMax > 0 {
		cfg.Temp.Max = generateTempMax
	}

	// Environment variable overrides
	if env := os.Getenv("KQL_VALIDATE"); env == "false" || env == "0" {
		cfg.Enabled = false
	}
	if env := os.Getenv("KQL_VALIDATE_STRICT"); env == "true" || env == "1" {
		cfg.Strict = true
	}
	// Add more env var handling as needed...

	return cfg
}

func buildGeneratePrompt(description, table, schema string) string {
	var context strings.Builder

	context.WriteString(`You are a Kusto Query Language (KQL) expert. Generate a KQL query based on the user's natural language description.

Rules:
1. Output ONLY the raw KQL query, no explanations
2. Do NOT wrap the query in backticks or code blocks
3. Use proper KQL syntax and operators
4. Include comments only if the query is complex
5. Prefer efficient query patterns
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

	// Check for markdown code blocks (triple backticks)
	if strings.Contains(response, "```") {
		// Try to find kql/kusto code block first
		for _, lang := range []string{"```kql", "```kusto", "```"} {
			if idx := strings.Index(response, lang); idx != -1 {
				start := idx + len(lang)
				// Find the closing ```
				if end := strings.Index(response[start:], "```"); end != -1 {
					extracted := strings.TrimSpace(response[start : start+end])
					if extracted != "" {
						return stripInlineBackticks(extracted)
					}
				}
			}
		}
	}

	// Strip inline backticks (e.g., `query here`)
	response = stripInlineBackticks(response)

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

// stripInlineBackticks removes inline backticks from a string.
// Handles cases like `query here` or queries starting/ending with backticks.
func stripInlineBackticks(s string) string {
	s = strings.TrimSpace(s)

	// Remove surrounding single backticks
	if len(s) >= 2 && s[0] == '`' && s[len(s)-1] == '`' {
		s = s[1 : len(s)-1]
	}

	// Also handle case where only leading or trailing backtick
	s = strings.TrimPrefix(s, "`")
	s = strings.TrimSuffix(s, "`")

	return strings.TrimSpace(s)
}
