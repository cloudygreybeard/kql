// Copyright 2026 cloudygreybeard
// SPDX-License-Identifier: Apache-2.0

package ai

import (
	"context"
	"fmt"
	"os"
)

// VertexProvider implements the Provider interface for Google Vertex AI.
// Supports Gemini and Claude models via the Vertex AI Model Garden.
//
// Requires: cloud.google.com/go/vertexai/genai
type VertexProvider struct {
	project     string
	location    string
	model       string
	temperature float32
	client      vertexClient
}

// vertexClient abstracts the Vertex AI client for testing.
type vertexClient interface {
	GenerateContent(ctx context.Context, prompt string, temp float32) (string, error)
	Close() error
}

// NewVertexProvider creates a new Vertex AI provider.
func NewVertexProvider(cfg Config) (*VertexProvider, error) {
	project := cfg.Vertex.Project
	if project == "" {
		project = os.Getenv("KQL_GCP_PROJECT")
	}
	if project == "" {
		project = os.Getenv("GOOGLE_CLOUD_PROJECT")
	}
	if project == "" {
		return nil, fmt.Errorf("vertex: project required (set --vertex-project, KQL_GCP_PROJECT, or GOOGLE_CLOUD_PROJECT)")
	}

	location := cfg.Vertex.Location
	if location == "" {
		location = DefaultVertexLocation
	}

	model := cfg.Model
	if model == "" {
		model = DefaultVertexModel
	}

	// Create the actual client
	client, err := newVertexGenAIClient(context.Background(), project, location, model)
	if err != nil {
		return nil, fmt.Errorf("vertex: creating client: %w", err)
	}

	return &VertexProvider{
		project:     project,
		location:    location,
		model:       model,
		temperature: cfg.Temperature,
		client:      client,
	}, nil
}

// Name returns the provider name.
func (p *VertexProvider) Name() string {
	return "vertex"
}

// Model returns the model name.
func (p *VertexProvider) Model() string {
	return p.model
}

// Complete sends a prompt and returns the response.
func (p *VertexProvider) Complete(ctx context.Context, prompt string) (string, error) {
	return p.client.GenerateContent(ctx, prompt, p.temperature)
}

// CompleteChat sends a chat conversation and returns the response.
func (p *VertexProvider) CompleteChat(ctx context.Context, messages []Message) (string, error) {
	// For now, concatenate messages into a single prompt
	// TODO: Use proper chat API when available
	var prompt string
	for _, m := range messages {
		switch m.Role {
		case RoleSystem:
			prompt += "System: " + m.Content + "\n\n"
		case RoleUser:
			prompt += "User: " + m.Content + "\n\n"
		case RoleAssistant:
			prompt += "Assistant: " + m.Content + "\n\n"
		}
	}
	prompt += "Assistant: "
	return p.Complete(ctx, prompt)
}

// Close closes the Vertex AI client.
func (p *VertexProvider) Close() error {
	if p.client != nil {
		return p.client.Close()
	}
	return nil
}
