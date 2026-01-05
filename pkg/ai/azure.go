// Copyright 2026 cloudygreybeard
// SPDX-License-Identifier: Apache-2.0

package ai

import (
	"context"
	"fmt"
	"os"
)

// AzureProvider implements the Provider interface for Azure OpenAI.
//
// Requires: github.com/Azure/azure-sdk-for-go/sdk/ai/azopenai
type AzureProvider struct {
	endpoint    string
	deployment  string
	model       string
	temperature float32
	client      azureClient
}

// azureClient abstracts the Azure OpenAI client for testing.
type azureClient interface {
	ChatComplete(ctx context.Context, messages []Message, temp float32) (string, error)
}

// NewAzureProvider creates a new Azure OpenAI provider.
func NewAzureProvider(cfg Config) (*AzureProvider, error) {
	endpoint := cfg.Azure.Endpoint
	if endpoint == "" {
		endpoint = os.Getenv("AZURE_OPENAI_ENDPOINT")
	}
	if endpoint == "" {
		return nil, fmt.Errorf("azure: endpoint required (set --azure-endpoint or AZURE_OPENAI_ENDPOINT)")
	}

	deployment := cfg.Azure.Deployment
	if deployment == "" {
		deployment = os.Getenv("AZURE_OPENAI_DEPLOYMENT")
	}
	if deployment == "" {
		return nil, fmt.Errorf("azure: deployment required (set --azure-deployment or AZURE_OPENAI_DEPLOYMENT)")
	}

	apiKey := cfg.Azure.APIKey
	if apiKey == "" {
		apiKey = os.Getenv("AZURE_OPENAI_API_KEY")
	}

	model := cfg.Model
	if model == "" {
		model = "gpt-4o"
	}

	// Create the actual client
	client, err := newAzureOpenAIClient(endpoint, deployment, apiKey)
	if err != nil {
		return nil, fmt.Errorf("azure: creating client: %w", err)
	}

	return &AzureProvider{
		endpoint:    endpoint,
		deployment:  deployment,
		model:       model,
		temperature: cfg.Temperature,
		client:      client,
	}, nil
}

// Name returns the provider name.
func (p *AzureProvider) Name() string {
	return "azure"
}

// Model returns the model name.
func (p *AzureProvider) Model() string {
	return p.model
}

// Complete sends a prompt and returns the response.
func (p *AzureProvider) Complete(ctx context.Context, prompt string) (string, error) {
	return p.CompleteChat(ctx, []Message{{Role: RoleUser, Content: prompt}})
}

// CompleteChat sends a chat conversation and returns the response.
func (p *AzureProvider) CompleteChat(ctx context.Context, messages []Message) (string, error) {
	return p.client.ChatComplete(ctx, messages, p.temperature)
}

