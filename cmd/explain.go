// Copyright 2026 cloudygreybeard
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/cloudygreybeard/kql/pkg/ai"
	"github.com/cloudygreybeard/kqlparser"
	"github.com/spf13/cobra"
)

var (
	// AI provider flags
	aiProvider       string
	aiModel          string
	aiTemperature    float32
	ollamaEndpoint   string
	vertexProject    string
	vertexLocation   string
	azureEndpoint    string
	azureDeployment  string
	instructEndpoint string

	// Explain-specific flags
	explainInputFile string
	explainVerbose   bool
	explainTimeout   int
)

var explainCmd = &cobra.Command{
	Use:   "explain [query]",
	Short: "Explain a KQL query in natural language",
	Long: `Explain a KQL query using an AI model.

The query can be provided as an argument, from a file (-f), or via stdin.

Supported AI providers:
  - ollama:      Local Ollama instance (default)
  - instructlab: Local InstructLab instance
  - vertex:      Google Vertex AI (Gemini, Claude)
  - azure:       Azure OpenAI

Configuration can be provided via:
  - Command-line flags
  - Environment variables (KQL_AI_PROVIDER, KQL_GCP_PROJECT, etc.)
  - Config file (~/.kql/config.yaml)`,
	Example: `  # Explain a simple query (using local Ollama)
  kql explain "StormEvents | summarize count() by State"

  # Explain from a file
  kql explain -f query.kql

  # Use a specific provider
  kql explain --provider vertex --model gemini-1.5-pro "T | take 10"

  # Use Azure OpenAI
  kql explain --provider azure --azure-endpoint https://myorg.openai.azure.com "T | take 10"`,
	RunE: runExplain,
}

func init() {
	rootCmd.AddCommand(explainCmd)

	// Provider selection
	explainCmd.Flags().StringVar(&aiProvider, "provider", "", "AI provider (ollama, instructlab, vertex, azure)")
	explainCmd.Flags().StringVar(&aiModel, "model", "", "Model name")
	explainCmd.Flags().Float32Var(&aiTemperature, "temperature", 0.2, "Temperature (0.0-1.0)")

	// Ollama
	explainCmd.Flags().StringVar(&ollamaEndpoint, "ollama-endpoint", "", "Ollama endpoint URL")

	// Vertex AI
	explainCmd.Flags().StringVar(&vertexProject, "vertex-project", "", "GCP project ID")
	explainCmd.Flags().StringVar(&vertexLocation, "vertex-location", "", "GCP location")

	// Azure OpenAI
	explainCmd.Flags().StringVar(&azureEndpoint, "azure-endpoint", "", "Azure OpenAI endpoint URL")
	explainCmd.Flags().StringVar(&azureDeployment, "azure-deployment", "", "Azure OpenAI deployment name")

	// InstructLab
	explainCmd.Flags().StringVar(&instructEndpoint, "instructlab-endpoint", "", "InstructLab endpoint URL")

	// Command options
	explainCmd.Flags().StringVarP(&explainInputFile, "file", "f", "", "Read query from file")
	explainCmd.Flags().BoolVarP(&explainVerbose, "verbose", "v", false, "Show additional context")
	explainCmd.Flags().IntVar(&explainTimeout, "timeout", 60, "Timeout in seconds")
}

func runExplain(cmd *cobra.Command, args []string) error {
	// Get query input
	query, err := getInputFrom(args, explainInputFile, os.Stdin, isTerminal)
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

	// Optionally parse the query first for context
	var parseContext string
	if explainVerbose {
		parseContext = getParseContext(query)
	}

	// Build prompt
	prompt := buildExplainPrompt(query, parseContext)

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(explainTimeout)*time.Second)
	defer cancel()

	// Show progress
	if explainVerbose {
		fmt.Fprintf(os.Stderr, "Using %s provider with model %s...\n", provider.Name(), provider.Model())
	}

	// Get explanation
	explanation, err := provider.Complete(ctx, prompt)
	if err != nil {
		return fmt.Errorf("getting explanation: %w", err)
	}

	fmt.Println(explanation)
	return nil
}

func buildAIConfig() ai.Config {
	// Start with defaults to ensure Validation config is initialized
	cfg := ai.DefaultConfig()

	// Override with flag values (empty strings/zero values are handled by MergeFileConfig)
	cfg.Provider = aiProvider
	cfg.Model = aiModel
	cfg.Temperature = aiTemperature
	cfg.Ollama.Endpoint = ollamaEndpoint
	cfg.Vertex.Project = vertexProject
	cfg.Vertex.Location = vertexLocation
	cfg.Azure.Endpoint = azureEndpoint
	cfg.Azure.Deployment = azureDeployment
	cfg.InstructLab.Endpoint = instructEndpoint

	return cfg
}

func getParseContext(query string) string {
	result := kqlparser.Parse("input", query)
	if len(result.Errors) > 0 {
		return fmt.Sprintf("Note: Query has %d syntax issue(s).", len(result.Errors))
	}
	return "Query syntax is valid."
}

func buildExplainPrompt(query, parseContext string) string {
	prompt := `You are a Kusto Query Language (KQL) expert. Explain the following KQL query in clear, concise terms.

Describe:
1. What data sources the query uses
2. Any filtering or transformations applied
3. The aggregations or computations performed
4. What the output will look like

Keep the explanation accessible to someone familiar with SQL but new to KQL.`

	if parseContext != "" {
		prompt += "\n\n" + parseContext
	}

	prompt += "\n\nQuery:\n```kql\n" + query + "\n```"

	return prompt
}
