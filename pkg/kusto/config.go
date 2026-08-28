// Copyright 2026 cloudygreybeard
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package kusto

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"gopkg.in/yaml.v3"
)

// FileConfig holds the sections of the kql configuration file this package
// reads. Unrelated sections are ignored, so each package decodes only what it
// owns rather than sharing one structure.
type FileConfig struct {
	// Clusters maps an alias to any cluster reference [ResolveCluster]
	// accepts.
	Clusters map[string]string `yaml:"clusters"`
	// Query supplies defaults for the query command.
	Query QueryFileConfig `yaml:"query"`
}

// QueryFileConfig holds defaults for the query command.
type QueryFileConfig struct {
	// Cluster is the cluster used when none is given on the command line.
	Cluster string `yaml:"cluster"`
	// Database is the database used when none is given on the command line.
	Database string `yaml:"database"`
	// Format is the default output format.
	Format string `yaml:"format"`
	// Timeout is the default server-side timeout, as a Go duration. An empty
	// value leaves the cluster's own default in force.
	Timeout string `yaml:"timeout"`
	// Auth names the default credential source: auto, az, exec or env.
	Auth string `yaml:"auth"`
	// AuthCommand is a credential helper and its arguments, used when Auth
	// selects it. The elements are passed to the operating system verbatim:
	// no shell interprets them, and kql substitutes nothing into them.
	AuthCommand []string `yaml:"auth_command"`
	// AuthEnv holds additional environment variables for the helper.
	AuthEnv map[string]string `yaml:"auth_env"`
}

// ErrConfigWritable reports that the configuration file, or the directory
// holding it, may be modified by users other than its owner.
var ErrConfigWritable = errors.New("configuration file is writable by others")

// groupOrWorldWritable is the set of permission bits granting write access to
// users other than the owner.
const groupOrWorldWritable = 0o022

// checkTrusted verifies that path, and the directory holding it, cannot be
// modified by users other than the owner.
//
// It applies only where a credential helper is configured. Anyone able to
// write the file could otherwise nominate a command that kql will run, so the
// file must be no less protected than the credentials it reaches. OpenSSH
// applies the same reasoning to ~/.ssh/config under StrictModes.
//
// Permission bits are synthesised on Windows and do not describe the access
// control actually in force, so the check is confined to systems where they
// are meaningful.
func checkTrusted(path string) error {
	if runtime.GOOS == "windows" {
		return nil
	}

	for _, p := range []string{path, filepath.Dir(path)} {
		info, err := os.Stat(p)
		if err != nil {
			return fmt.Errorf("checking permissions of %s: %w", p, err)
		}
		if mode := info.Mode().Perm(); mode&groupOrWorldWritable != 0 {
			return fmt.Errorf("%w: %s has mode %#o and names a credential helper in query.auth_command; run 'chmod go-w %s'",
				ErrConfigWritable, p, mode, p)
		}
	}
	return nil
}

// ConfigPath returns the location of the kql configuration file.
func ConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locating home directory: %w", err)
	}
	return filepath.Join(home, ".kql", "config.yaml"), nil
}

// LoadConfig reads the kql configuration file, returning a zero value when the
// file is absent.
func LoadConfig() (*FileConfig, error) {
	path, err := ConfigPath()
	if err != nil {
		return &FileConfig{}, err
	}
	return LoadConfigFromPath(path)
}

// LoadConfigFromPath reads configuration from path, returning a zero value
// when the file is absent.
func LoadConfigFromPath(path string) (*FileConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &FileConfig{}, nil
		}
		return &FileConfig{}, fmt.Errorf("reading %s: %w", path, err)
	}

	var cfg FileConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return &FileConfig{}, fmt.Errorf("parsing %s: %w", path, err)
	}

	if len(cfg.Query.AuthCommand) > 0 {
		if err := checkTrusted(path); err != nil {
			return &FileConfig{}, err
		}
	}
	return &cfg, nil
}
