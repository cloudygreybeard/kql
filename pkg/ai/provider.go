// Copyright 2026 cloudygreybeard
// SPDX-License-Identifier: Apache-2.0

// Package ai provides a multi-provider abstraction for LLM integration.
// Supported providers include Vertex AI, Azure OpenAI, Ollama, and InstructLab.
package ai

import (
	"context"
	"fmt"
)

// Default configuration values.
const (
	// DefaultProvider is the default AI provider.
	DefaultProvider = "ollama"

	// DefaultTemperature is the default temperature for generation.
	DefaultTemperature = 0.2

	// Ollama defaults
	DefaultOllamaHost     = "localhost"
	DefaultOllamaPort     = "11434"
	DefaultOllamaEndpoint = "http://" + DefaultOllamaHost + ":" + DefaultOllamaPort
	DefaultOllamaModel    = "llama3.2"

	// InstructLab defaults
	DefaultInstructLabHost     = "localhost"
	DefaultInstructLabPort     = "8000"
	DefaultInstructLabEndpoint = "http://" + DefaultInstructLabHost + ":" + DefaultInstructLabPort
	DefaultInstructLabModel    = "default"

	// Vertex AI defaults
	DefaultVertexLocation = "us-east5"         // us-east5 required for Claude models
	DefaultVertexModel    = "claude-opus-4-5"  // Claude 4.5 Opus via Model Garden

	// Azure defaults
	DefaultAzureModel = "gpt-4o"

	// Validation defaults
	DefaultValidationEnabled       = true
	DefaultValidationStrict        = false
	DefaultValidationRetries       = 2
	DefaultFeedbackErrors          = true
	DefaultFeedbackHints           = true
	DefaultFeedbackExamples        = true
	DefaultFeedbackProgressive     = true
	DefaultRetryTempAdjust         = true
	DefaultRetryTempIncrement      = 0.1
	DefaultRetryTempMax    float32 = 0.8
)

// Provider defines the interface for AI/LLM providers.
type Provider interface {
	// Complete sends a prompt and returns the model's response.
	Complete(ctx context.Context, prompt string) (string, error)

	// CompleteChat sends a conversation and returns the model's response.
	CompleteChat(ctx context.Context, messages []Message) (string, error)

	// Name returns the provider's identifier.
	Name() string

	// Model returns the model being used.
	Model() string
}

// Message represents a chat message.
type Message struct {
	Role    Role
	Content string
}

// Role represents the role of a message sender.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// Config holds configuration for AI providers.
type Config struct {
	// Provider name: "ollama", "vertex", "azure", "instructlab", "openai", "anthropic"
	Provider string

	// Model name (provider-specific)
	Model string

	// Temperature controls randomness (0.0-1.0)
	Temperature float32

	// Ollama configuration
	Ollama OllamaConfig

	// Vertex AI configuration
	Vertex VertexConfig

	// Azure OpenAI configuration
	Azure AzureConfig

	// InstructLab configuration
	InstructLab InstructLabConfig

	// Validation configuration for generated output
	Validation ValidationConfig
}

// OllamaConfig holds Ollama-specific configuration.
type OllamaConfig struct {
	// Endpoint URL (default: http://localhost:11434)
	Endpoint string
}

// VertexConfig holds Vertex AI-specific configuration.
type VertexConfig struct {
	// GCP Project ID
	Project string

	// GCP Location (default: us-central1)
	Location string
}

// AzureConfig holds Azure OpenAI-specific configuration.
type AzureConfig struct {
	// Azure OpenAI endpoint URL
	Endpoint string

	// Deployment name
	Deployment string

	// API Key (optional, uses Azure AD if not set)
	APIKey string
}

// InstructLabConfig holds InstructLab-specific configuration.
type InstructLabConfig struct {
	// Endpoint URL (default: http://localhost:8000)
	Endpoint string
}

// ValidationConfig holds validation and retry settings for AI-generated output.
type ValidationConfig struct {
	// Enabled enables validation of generated KQL (default: true)
	Enabled bool

	// Strict fails with exit code 1 if validation fails (default: false)
	Strict bool

	// Retries is the number of retry attempts on validation failure (default: 2)
	Retries int

	// Feedback controls what information is included in retry prompts
	Feedback FeedbackConfig

	// Temp controls temperature adjustment on retries
	Temp TempAdjustConfig
}

// FeedbackConfig controls what feedback is included in retry prompts.
type FeedbackConfig struct {
	// Errors includes the specific error messages (default: true)
	Errors bool

	// Hints includes contextual hints for error types (default: true)
	Hints bool

	// Examples includes syntax examples (default: true)
	Examples bool

	// Progressive increases detail with each retry (default: true)
	Progressive bool
}

// TempAdjustConfig controls temperature adjustment on retries.
type TempAdjustConfig struct {
	// Adjust enables temperature adjustment (default: true)
	Adjust bool

	// Increment is the temperature increase per retry (default: 0.1)
	Increment float32

	// Max caps the adjusted temperature (default: 0.8)
	Max float32
}

// DefaultValidationConfig returns validation config with sensible defaults.
func DefaultValidationConfig() ValidationConfig {
	return ValidationConfig{
		Enabled: DefaultValidationEnabled,
		Strict:  DefaultValidationStrict,
		Retries: DefaultValidationRetries,
		Feedback: FeedbackConfig{
			Errors:      DefaultFeedbackErrors,
			Hints:       DefaultFeedbackHints,
			Examples:    DefaultFeedbackExamples,
			Progressive: DefaultFeedbackProgressive,
		},
		Temp: TempAdjustConfig{
			Adjust:    DefaultRetryTempAdjust,
			Increment: DefaultRetryTempIncrement,
			Max:       DefaultRetryTempMax,
		},
	}
}

// NewProvider creates a provider based on the configuration.
func NewProvider(cfg Config) (Provider, error) {
	switch cfg.Provider {
	case "ollama":
		return NewOllamaProvider(cfg)
	case "instructlab":
		return NewInstructLabProvider(cfg)
	case "vertex":
		return NewVertexProvider(cfg)
	case "azure":
		return NewAzureProvider(cfg)
	default:
		return nil, fmt.Errorf("unknown provider: %q (supported: ollama, instructlab, vertex, azure)", cfg.Provider)
	}
}

// DefaultConfig returns a configuration with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Provider:    DefaultProvider,
		Model:       DefaultOllamaModel,
		Temperature: DefaultTemperature,
		Ollama: OllamaConfig{
			Endpoint: DefaultOllamaEndpoint,
		},
		Vertex: VertexConfig{
			Location: DefaultVertexLocation,
		},
		InstructLab: InstructLabConfig{
			Endpoint: DefaultInstructLabEndpoint,
		},
		Validation: DefaultValidationConfig(),
	}
}
