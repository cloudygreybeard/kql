// Copyright 2026 cloudygreybeard
// SPDX-License-Identifier: Apache-2.0

package ai

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// FileConfig represents the configuration file structure.
type FileConfig struct {
	AI AIFileConfig `yaml:"ai"`
}

// AIFileConfig represents the AI section of the configuration file.
type AIFileConfig struct {
	Provider    string  `yaml:"provider"`
	Model       string  `yaml:"model"`
	Temperature float32 `yaml:"temperature"`

	Ollama struct {
		Endpoint string `yaml:"endpoint"`
	} `yaml:"ollama"`

	Vertex struct {
		Project  string `yaml:"project"`
		Location string `yaml:"location"`
	} `yaml:"vertex"`

	Azure struct {
		Endpoint   string `yaml:"endpoint"`
		Deployment string `yaml:"deployment"`
		APIKey     string `yaml:"api_key"`
	} `yaml:"azure"`

	InstructLab struct {
		Endpoint string `yaml:"endpoint"`
	} `yaml:"instructlab"`
}

// LoadConfigFile loads configuration from ~/.kql/config.yaml if it exists.
func LoadConfigFile() (*FileConfig, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	configPath := filepath.Join(home, ".kql", "config.yaml")
	return LoadConfigFromPath(configPath)
}

// LoadConfigFromPath loads configuration from a specific path.
func LoadConfigFromPath(path string) (*FileConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No config file is OK
		}
		return nil, err
	}

	var cfg FileConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// MergeFileConfig merges file configuration into a Config, with file config as defaults.
func MergeFileConfig(cfg Config, fileCfg *FileConfig) Config {
	if fileCfg == nil {
		return cfg
	}

	ai := fileCfg.AI

	// Provider (file config is default, can be overridden)
	if cfg.Provider == "" && ai.Provider != "" {
		cfg.Provider = ai.Provider
	}

	// Model
	if cfg.Model == "" && ai.Model != "" {
		cfg.Model = ai.Model
	}

	// Temperature (0 means use file config)
	if cfg.Temperature == 0 && ai.Temperature != 0 {
		cfg.Temperature = ai.Temperature
	}

	// Ollama
	if cfg.Ollama.Endpoint == "" && ai.Ollama.Endpoint != "" {
		cfg.Ollama.Endpoint = ai.Ollama.Endpoint
	}

	// Vertex
	if cfg.Vertex.Project == "" && ai.Vertex.Project != "" {
		cfg.Vertex.Project = ai.Vertex.Project
	}
	if cfg.Vertex.Location == "" && ai.Vertex.Location != "" {
		cfg.Vertex.Location = ai.Vertex.Location
	}

	// Azure
	if cfg.Azure.Endpoint == "" && ai.Azure.Endpoint != "" {
		cfg.Azure.Endpoint = ai.Azure.Endpoint
	}
	if cfg.Azure.Deployment == "" && ai.Azure.Deployment != "" {
		cfg.Azure.Deployment = ai.Azure.Deployment
	}
	if cfg.Azure.APIKey == "" && ai.Azure.APIKey != "" {
		cfg.Azure.APIKey = ai.Azure.APIKey
	}

	// InstructLab
	if cfg.InstructLab.Endpoint == "" && ai.InstructLab.Endpoint != "" {
		cfg.InstructLab.Endpoint = ai.InstructLab.Endpoint
	}

	return cfg
}
