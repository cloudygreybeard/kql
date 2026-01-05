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

// InstructLabProvider implements the Provider interface for InstructLab.
// InstructLab uses an OpenAI-compatible API.
type InstructLabProvider struct {
	endpoint    string
	model       string
	temperature float32
	client      *http.Client
}

// NewInstructLabProvider creates a new InstructLab provider.
func NewInstructLabProvider(cfg Config) (*InstructLabProvider, error) {
	endpoint := cfg.InstructLab.Endpoint
	if endpoint == "" {
		endpoint = "http://localhost:8000"
	}

	model := cfg.Model
	if model == "" {
		model = "default"
	}

	return &InstructLabProvider{
		endpoint:    strings.TrimSuffix(endpoint, "/"),
		model:       model,
		temperature: cfg.Temperature,
		client:      &http.Client{},
	}, nil
}

// Name returns the provider name.
func (p *InstructLabProvider) Name() string {
	return "instructlab"
}

// Model returns the model name.
func (p *InstructLabProvider) Model() string {
	return p.model
}

// Complete sends a prompt and returns the response.
func (p *InstructLabProvider) Complete(ctx context.Context, prompt string) (string, error) {
	return p.CompleteChat(ctx, []Message{{Role: RoleUser, Content: prompt}})
}

// CompleteChat sends a chat conversation and returns the response.
// Uses OpenAI-compatible API format.
func (p *InstructLabProvider) CompleteChat(ctx context.Context, messages []Message) (string, error) {
	// Convert to OpenAI chat format
	openaiMessages := make([]openaiChatMessage, len(messages))
	for i, m := range messages {
		openaiMessages[i] = openaiChatMessage{
			Role:    string(m.Role),
			Content: m.Content,
		}
	}

	reqBody := openaiChatRequest{
		Model:       p.model,
		Messages:    openaiMessages,
		Temperature: p.temperature,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("sending request to instructlab: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("instructlab returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var result openaiChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decoding response: %w", err)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	return result.Choices[0].Message.Content, nil
}

// OpenAI-compatible API types (used by InstructLab)

type openaiChatRequest struct {
	Model       string              `json:"model"`
	Messages    []openaiChatMessage `json:"messages"`
	Temperature float32             `json:"temperature,omitempty"`
}

type openaiChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openaiChatResponse struct {
	Choices []openaiChoice `json:"choices"`
}

type openaiChoice struct {
	Message openaiChatMessage `json:"message"`
}

