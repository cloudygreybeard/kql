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
	DefaultVertexLocation = "us-central1"
	DefaultVertexModel    = "gemini-1.5-flash"

	// Azure defaults
	DefaultAzureModel = "gpt-4o"
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
	}
}
