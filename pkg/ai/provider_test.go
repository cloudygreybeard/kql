// Copyright 2026 cloudygreybeard
// SPDX-License-Identifier: Apache-2.0

package ai

import (
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Provider != DefaultProvider {
		t.Errorf("expected provider %q, got %q", DefaultProvider, cfg.Provider)
	}
	if cfg.Model != DefaultOllamaModel {
		t.Errorf("expected model %q, got %q", DefaultOllamaModel, cfg.Model)
	}
	if cfg.Temperature != DefaultTemperature {
		t.Errorf("expected temperature %f, got %f", DefaultTemperature, cfg.Temperature)
	}
	if cfg.Ollama.Endpoint != DefaultOllamaEndpoint {
		t.Errorf("expected ollama endpoint %q, got %q", DefaultOllamaEndpoint, cfg.Ollama.Endpoint)
	}
}

func TestNewProvider_UnknownProvider(t *testing.T) {
	cfg := Config{Provider: "unknown"}
	_, err := NewProvider(cfg)
	if err == nil {
		t.Error("expected error for unknown provider")
	}
}

func TestNewOllamaProvider(t *testing.T) {
	cfg := Config{
		Provider:    "ollama",
		Model:       "test-model",
		Temperature: 0.5,
		Ollama: OllamaConfig{
			Endpoint: "http://custom:1234",
		},
	}

	p, err := NewOllamaProvider(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if p.Name() != "ollama" {
		t.Errorf("expected name 'ollama', got %q", p.Name())
	}
	if p.Model() != "test-model" {
		t.Errorf("expected model 'test-model', got %q", p.Model())
	}
}

func TestNewInstructLabProvider(t *testing.T) {
	cfg := Config{
		Provider:    "instructlab",
		Model:       "kql-expert",
		Temperature: 0.3,
		InstructLab: InstructLabConfig{
			Endpoint: "http://localhost:8000",
		},
	}

	p, err := NewInstructLabProvider(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if p.Name() != "instructlab" {
		t.Errorf("expected name 'instructlab', got %q", p.Name())
	}
	if p.Model() != "kql-expert" {
		t.Errorf("expected model 'kql-expert', got %q", p.Model())
	}
}

func TestMergeFileConfig(t *testing.T) {
	fileCfg := &FileConfig{
		AI: AIFileConfig{
			Provider:    "vertex",
			Model:       "gemini-1.5-pro",
			Temperature: 0.3,
		},
	}
	fileCfg.AI.Vertex.Project = "my-project"
	fileCfg.AI.Vertex.Location = "us-east1"

	// Empty config should get file values
	cfg := Config{}
	merged := MergeFileConfig(cfg, fileCfg)

	if merged.Provider != "vertex" {
		t.Errorf("expected provider 'vertex', got %q", merged.Provider)
	}
	if merged.Model != "gemini-1.5-pro" {
		t.Errorf("expected model 'gemini-1.5-pro', got %q", merged.Model)
	}
	if merged.Temperature != 0.3 {
		t.Errorf("expected temperature 0.3, got %f", merged.Temperature)
	}
	if merged.Vertex.Project != "my-project" {
		t.Errorf("expected vertex project 'my-project', got %q", merged.Vertex.Project)
	}

	// CLI values should override file values
	cfg = Config{
		Provider: "ollama",
		Model:    "llama3.2",
	}
	merged = MergeFileConfig(cfg, fileCfg)

	if merged.Provider != "ollama" {
		t.Errorf("expected provider 'ollama', got %q", merged.Provider)
	}
	if merged.Model != "llama3.2" {
		t.Errorf("expected model 'llama3.2', got %q", merged.Model)
	}
}

func TestMergeFileConfig_NilFileConfig(t *testing.T) {
	cfg := Config{
		Provider: "ollama",
		Model:    "test",
	}

	merged := MergeFileConfig(cfg, nil)

	if merged.Provider != "ollama" {
		t.Errorf("expected provider 'ollama', got %q", merged.Provider)
	}
}
