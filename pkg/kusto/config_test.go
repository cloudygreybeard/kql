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
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// writeConfig writes body to a file in a new directory and returns its path.
func writeConfig(t *testing.T, body string) string {
	t.Helper()

	dir := filepath.Join(t.TempDir(), ".kql")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatalf("creating configuration directory: %v", err)
	}
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("writing configuration: %v", err)
	}
	return path
}

func TestLoadConfigFromPathAbsent(t *testing.T) {
	got, err := LoadConfigFromPath(filepath.Join(t.TempDir(), "absent.yaml"))
	if err != nil {
		t.Fatalf("LoadConfigFromPath() returned unexpected error: %v", err)
	}
	if got.Query.Cluster != "" || len(got.Clusters) != 0 {
		t.Errorf("LoadConfigFromPath() = %+v, want the zero value", got)
	}
}

func TestLoadConfigFromPathQuerySection(t *testing.T) {
	path := writeConfig(t, `
clusters:
  eu: https://help.kusto.windows.net
query:
  database: Samples
  format: json
  timeout: 90s
  auth: exec
  auth_command:
    - /opt/bin/broker
    - --audience-from-env
  auth_env:
    BROKER_PROFILE: work
`)

	got, err := LoadConfigFromPath(path)
	if err != nil {
		t.Fatalf("LoadConfigFromPath() returned unexpected error: %v", err)
	}

	if want := "https://help.kusto.windows.net"; got.Clusters["eu"] != want {
		t.Errorf("Clusters[eu] = %q, want %q", got.Clusters["eu"], want)
	}
	if want := "exec"; got.Query.Auth != want {
		t.Errorf("Query.Auth = %q, want %q", got.Query.Auth, want)
	}
	if want := []string{"/opt/bin/broker", "--audience-from-env"}; !equalStrings(got.Query.AuthCommand, want) {
		t.Errorf("Query.AuthCommand = %v, want %v", got.Query.AuthCommand, want)
	}
	if want := "work"; got.Query.AuthEnv["BROKER_PROFILE"] != want {
		t.Errorf("Query.AuthEnv[BROKER_PROFILE] = %q, want %q", got.Query.AuthEnv["BROKER_PROFILE"], want)
	}
}

func TestLoadConfigFromPathRejectsWritableHelperConfig(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits are synthesised on Windows")
	}

	path := writeConfig(t, "query:\n  auth_command: [/opt/bin/broker]\n")
	if err := os.Chmod(path, 0o666); err != nil {
		t.Fatalf("relaxing permissions: %v", err)
	}

	_, err := LoadConfigFromPath(path)
	if !errors.Is(err, ErrConfigWritable) {
		t.Fatalf("LoadConfigFromPath() error = %v, want ErrConfigWritable", err)
	}
}

func TestLoadConfigFromPathRejectsWritableHelperDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits are synthesised on Windows")
	}

	// A writable directory is as good as a writable file: the file can simply
	// be replaced.
	path := writeConfig(t, "query:\n  auth_command: [/opt/bin/broker]\n")
	if err := os.Chmod(filepath.Dir(path), 0o777); err != nil {
		t.Fatalf("relaxing permissions: %v", err)
	}

	_, err := LoadConfigFromPath(path)
	if !errors.Is(err, ErrConfigWritable) {
		t.Fatalf("LoadConfigFromPath() error = %v, want ErrConfigWritable", err)
	}
}

func TestLoadConfigFromPathAllowsWritableConfigWithoutHelper(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits are synthesised on Windows")
	}

	// The check exists to protect the helper. Without one, a relaxed mode is
	// the user's own affair and must not stop them querying.
	path := writeConfig(t, "query:\n  database: Samples\n")
	if err := os.Chmod(path, 0o666); err != nil {
		t.Fatalf("relaxing permissions: %v", err)
	}

	got, err := LoadConfigFromPath(path)
	if err != nil {
		t.Fatalf("LoadConfigFromPath() returned unexpected error: %v", err)
	}
	if want := "Samples"; got.Query.Database != want {
		t.Errorf("Query.Database = %q, want %q", got.Query.Database, want)
	}
}

// equalStrings reports whether a and b hold the same elements in order.
func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
