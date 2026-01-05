// Copyright 2026 cloudygreybeard
// SPDX-License-Identifier: Apache-2.0

package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// OllamaProvider implements the Provider interface for Ollama.
type OllamaProvider struct {
	endpoint    string
	model       string
	temperature float32
	client      *http.Client
}

// NewOllamaProvider creates a new Ollama provider.
func NewOllamaProvider(cfg Config) (*OllamaProvider, error) {
	endpoint := cfg.Ollama.Endpoint
	if endpoint == "" {
		endpoint = "http://localhost:11434"
	}

	model := cfg.Model
	if model == "" {
		model = "llama3.2"
	}

	return &OllamaProvider{
		endpoint:    strings.TrimSuffix(endpoint, "/"),
		model:       model,
		temperature: cfg.Temperature,
		client:      &http.Client{},
	}, nil
}

// Name returns the provider name.
func (p *OllamaProvider) Name() string {
	return "ollama"
}

// Model returns the model name.
func (p *OllamaProvider) Model() string {
	return p.model
}

// Complete sends a prompt and returns the response.
func (p *OllamaProvider) Complete(ctx context.Context, prompt string) (string, error) {
	return p.CompleteChat(ctx, []Message{{Role: RoleUser, Content: prompt}})
}

// CompleteChat sends a chat conversation and returns the response.
func (p *OllamaProvider) CompleteChat(ctx context.Context, messages []Message) (string, error) {
	// Convert to Ollama chat format
	ollamaMessages := make([]ollamaChatMessage, len(messages))
	for i, m := range messages {
		ollamaMessages[i] = ollamaChatMessage{
			Role:    string(m.Role),
			Content: m.Content,
		}
	}

	reqBody := ollamaChatRequest{
		Model:    p.model,
		Messages: ollamaMessages,
		Stream:   false,
		Options: ollamaOptions{
			Temperature: p.temperature,
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("sending request to ollama: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ollama returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var result ollamaChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decoding response: %w", err)
	}

	return result.Message.Content, nil
}

// Ollama API types

type ollamaChatRequest struct {
	Model    string              `json:"model"`
	Messages []ollamaChatMessage `json:"messages"`
	Stream   bool                `json:"stream"`
	Options  ollamaOptions       `json:"options,omitempty"`
}

type ollamaChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaOptions struct {
	Temperature float32 `json:"temperature,omitempty"`
}

type ollamaChatResponse struct {
	Message ollamaChatMessage `json:"message"`
}

