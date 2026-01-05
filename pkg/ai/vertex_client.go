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
	"os/exec"
	"strings"
)

// vertexGenAIClient uses the Vertex AI REST API with gcloud auth.
type vertexGenAIClient struct {
	project   string
	location  string
	modelName string
	client    *http.Client
}

// newVertexGenAIClient creates a new Vertex AI client.
func newVertexGenAIClient(ctx context.Context, project, location, modelName string) (*vertexGenAIClient, error) {
	return &vertexGenAIClient{
		project:   project,
		location:  location,
		modelName: modelName,
		client:    &http.Client{},
	}, nil
}

// getAccessToken retrieves an access token using gcloud.
func (c *vertexGenAIClient) getAccessToken() (string, error) {
	cmd := exec.Command("gcloud", "auth", "print-access-token")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("getting access token (ensure gcloud is configured): %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// GenerateContent generates content using the Vertex AI model.
func (c *vertexGenAIClient) GenerateContent(ctx context.Context, prompt string, temp float32) (string, error) {
	token, err := c.getAccessToken()
	if err != nil {
		return "", err
	}

	// Detect Claude models (use Anthropic API format on Vertex)
	if c.isClaude() {
		return c.generateClaudeContent(ctx, token, prompt, temp)
	}

	return c.generateGeminiContent(ctx, token, prompt, temp)
}

// isClaude returns true if the model is a Claude model.
func (c *vertexGenAIClient) isClaude() bool {
	return strings.HasPrefix(c.modelName, "claude")
}

// generateGeminiContent uses the Gemini/PaLM API format.
func (c *vertexGenAIClient) generateGeminiContent(ctx context.Context, token, prompt string, temp float32) (string, error) {
	url := fmt.Sprintf(
		"https://%s-aiplatform.googleapis.com/v1/projects/%s/locations/%s/publishers/google/models/%s:generateContent",
		c.location, c.project, c.location, c.modelName,
	)

	reqBody := vertexRequest{
		Contents: []vertexContent{{
			Role: "user",
			Parts: []vertexPart{{
				Text: prompt,
			}},
		}},
		GenerationConfig: vertexGenerationConfig{
			Temperature: temp,
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("sending request to vertex: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("vertex returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var result vertexResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decoding response: %w", err)
	}

	if len(result.Candidates) == 0 {
		return "", fmt.Errorf("no candidates in response")
	}

	candidate := result.Candidates[0]
	if len(candidate.Content.Parts) == 0 {
		return "", fmt.Errorf("no parts in response")
	}

	return candidate.Content.Parts[0].Text, nil
}

// generateClaudeContent uses the Anthropic Messages API format on Vertex AI.
func (c *vertexGenAIClient) generateClaudeContent(ctx context.Context, token, prompt string, temp float32) (string, error) {
	// Claude on Vertex uses the Anthropic publisher endpoint
	url := fmt.Sprintf(
		"https://%s-aiplatform.googleapis.com/v1/projects/%s/locations/%s/publishers/anthropic/models/%s:rawPredict",
		c.location, c.project, c.location, c.modelName,
	)

	reqBody := claudeRequest{
		AnthropicVersion: "vertex-2023-10-16",
		Messages: []claudeMessage{{
			Role:    "user",
			Content: prompt,
		}},
		MaxTokens:   4096,
		Temperature: temp,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("sending request to vertex: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("vertex (claude) returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var result claudeResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decoding response: %w", err)
	}

	if len(result.Content) == 0 {
		return "", fmt.Errorf("no content in response")
	}

	return result.Content[0].Text, nil
}

// Close is a no-op for the HTTP-based client.
func (c *vertexGenAIClient) Close() error {
	return nil
}

// Vertex AI API types

type vertexRequest struct {
	Contents         []vertexContent        `json:"contents"`
	GenerationConfig vertexGenerationConfig `json:"generationConfig,omitempty"`
}

type vertexContent struct {
	Role  string       `json:"role"`
	Parts []vertexPart `json:"parts"`
}

type vertexPart struct {
	Text string `json:"text"`
}

type vertexGenerationConfig struct {
	Temperature float32 `json:"temperature,omitempty"`
}

type vertexResponse struct {
	Candidates []vertexCandidate `json:"candidates"`
}

type vertexCandidate struct {
	Content vertexContent `json:"content"`
}

// Claude API types (for Vertex AI Anthropic endpoint)

type claudeRequest struct {
	AnthropicVersion string          `json:"anthropic_version"`
	Messages         []claudeMessage `json:"messages"`
	MaxTokens        int             `json:"max_tokens"`
	Temperature      float32         `json:"temperature,omitempty"`
}

type claudeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type claudeResponse struct {
	Content []claudeContentBlock `json:"content"`
}

type claudeContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}
