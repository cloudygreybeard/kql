// Copyright 2024 cloudygreybeard
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

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestMain_Direct tests the main function by calling it directly with mocked args
func TestMain_Direct(t *testing.T) {
	// Save original args and restore after test
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	// Test with --help which doesn't exit
	os.Args = []string{"kql", "--help"}
	
	// main() will call cmd.Execute() which prints help and returns nil
	// This should not panic or exit
	main()
}

// Note: Testing main() error path is not possible without os.Exit mocking
// The error path (lines 27-28) calls os.Exit(1) which would terminate the test

// TestLintCommand_ExitCode tests lint command exit codes via subprocess
func TestLintCommand_ExitCode(t *testing.T) {
	// Build the binary first
	tmpDir := t.TempDir()
	binary := filepath.Join(tmpDir, "kql")

	// Build the binary
	buildCmd := exec.Command("go", "build", "-o", binary, ".")
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("failed to build binary: %v", err)
	}

	t.Run("valid_query_exit_0", func(t *testing.T) {
		cmd := exec.Command(binary, "lint")
		cmd.Stdin = strings.NewReader("T | take 10\n")
		err := cmd.Run()
		if err != nil {
			t.Errorf("expected exit 0 for valid query, got error: %v", err)
		}
	})

	t.Run("syntax_error_exit_1", func(t *testing.T) {
		cmd := exec.Command(binary, "lint")
		cmd.Stdin = strings.NewReader("T | where ((\n")
		err := cmd.Run()
		if err == nil {
			t.Error("expected exit 1 for syntax error")
		}
		// Verify it's an exit error with code 1
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() != 1 {
				t.Errorf("expected exit code 1, got %d", exitErr.ExitCode())
			}
		}
	})

	t.Run("lint_file", func(t *testing.T) {
		// Create a test file
		testFile := filepath.Join(tmpDir, "test.kql")
		if err := os.WriteFile(testFile, []byte("print 'hello'\n"), 0644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}

		cmd := exec.Command(binary, "lint", testFile)
		err := cmd.Run()
		if err != nil {
			t.Errorf("expected exit 0 for valid file, got error: %v", err)
		}
	})

	t.Run("lint_strict", func(t *testing.T) {
		cmd := exec.Command(binary, "lint", "--strict")
		cmd.Stdin = strings.NewReader("print 'hello'\n")
		err := cmd.Run()
		// Strict mode may produce warnings/errors for unresolved references
		// We just want to verify the command runs
		t.Logf("strict mode result: %v", err)
	})

	t.Run("lint_json_output", func(t *testing.T) {
		cmd := exec.Command(binary, "lint", "--format", "json")
		cmd.Stdin = strings.NewReader("T | where ((\n")
		output, _ := cmd.CombinedOutput()
		if !strings.Contains(string(output), `"severity":"error"`) {
			t.Errorf("expected JSON error output, got: %s", output)
		}
	})

	t.Run("lint_quiet", func(t *testing.T) {
		cmd := exec.Command(binary, "lint", "--quiet")
		cmd.Stdin = strings.NewReader("T | take 10\n")
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		// Quiet mode should produce no output for valid queries
		if strings.Contains(string(output), "No issues") {
			t.Error("quiet mode should not print success message")
		}
	})
}

// TestVersionCommand tests version command
func TestVersionCommand(t *testing.T) {
	tmpDir := t.TempDir()
	binary := filepath.Join(tmpDir, "kql")

	// Build the binary
	buildCmd := exec.Command("go", "build", "-o", binary, ".")
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("failed to build binary: %v", err)
	}

	cmd := exec.Command(binary, "version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Errorf("version command failed: %v", err)
	}
	if !strings.Contains(string(output), "kql version") {
		t.Errorf("expected version output, got: %s", output)
	}
}

// TestLinkCommands tests link build/extract commands
func TestLinkCommands(t *testing.T) {
	tmpDir := t.TempDir()
	binary := filepath.Join(tmpDir, "kql")

	// Build the binary
	buildCmd := exec.Command("go", "build", "-o", binary, ".")
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("failed to build binary: %v", err)
	}

	t.Run("link_build_extract_roundtrip", func(t *testing.T) {
		// Build a link
		buildCmd := exec.Command(binary, "link", "build", "-c", "help", "-d", "Samples")
		buildCmd.Stdin = strings.NewReader("print 'hello'\n")
		buildOutput, err := buildCmd.CombinedOutput()
		if err != nil {
			t.Fatalf("link build failed: %v, output: %s", err, buildOutput)
		}

		link := strings.TrimSpace(string(buildOutput))

		// Extract the query back
		extractCmd := exec.Command(binary, "link", "extract", link)
		extractOutput, err := extractCmd.CombinedOutput()
		if err != nil {
			t.Fatalf("link extract failed: %v, output: %s", err, extractOutput)
		}

		if !strings.Contains(string(extractOutput), "print") {
			t.Errorf("expected query to contain 'print', got: %s", extractOutput)
		}
	})
}

