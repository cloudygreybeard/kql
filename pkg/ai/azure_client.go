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
	"os"
	"strings"
)

// azureOpenAIClient uses the Azure OpenAI REST API directly.
type azureOpenAIClient struct {
	endpoint   string
	deployment string
	apiKey     string
	client     *http.Client
}

// newAzureOpenAIClient creates a new Azure OpenAI client.
func newAzureOpenAIClient(endpoint, deployment, apiKey string) (*azureOpenAIClient, error) {
	// If no API key provided, try to get from environment
	if apiKey == "" {
		apiKey = os.Getenv("AZURE_OPENAI_API_KEY")
	}

	if apiKey == "" {
		return nil, fmt.Errorf("azure: API key required (set --azure-api-key or AZURE_OPENAI_API_KEY)")
	}

	return &azureOpenAIClient{
		endpoint:   strings.TrimSuffix(endpoint, "/"),
		deployment: deployment,
		apiKey:     apiKey,
		client:     &http.Client{},
	}, nil
}

// ChatComplete sends a chat completion request.
func (c *azureOpenAIClient) ChatComplete(ctx context.Context, messages []Message, temp float32) (string, error) {
	// Convert messages to Azure format
	azureMessages := make([]azureChatMessage, len(messages))
	for i, m := range messages {
		azureMessages[i] = azureChatMessage{
			Role:    string(m.Role),
			Content: m.Content,
		}
	}

	reqBody := azureChatRequest{
		Messages:    azureMessages,
		Temperature: temp,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshaling request: %w", err)
	}

	// Azure OpenAI API endpoint format
	url := fmt.Sprintf("%s/openai/deployments/%s/chat/completions?api-version=2024-02-15-preview",
		c.endpoint, c.deployment)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("api-key", c.apiKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("sending request to azure: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("azure returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var result azureChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decoding response: %w", err)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	return result.Choices[0].Message.Content, nil
}

// Azure OpenAI API types

type azureChatRequest struct {
	Messages    []azureChatMessage `json:"messages"`
	Temperature float32            `json:"temperature,omitempty"`
}

type azureChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type azureChatResponse struct {
	Choices []azureChatChoice `json:"choices"`
}

type azureChatChoice struct {
	Message azureChatMessage `json:"message"`
}
