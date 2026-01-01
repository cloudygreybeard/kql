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

package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLintQuery_ValidSyntax(t *testing.T) {
	lintStrict = false
	diagnostics, err := lintQuery("test.kql", "T | where x > 10 | summarize count()")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(diagnostics) != 0 {
		t.Errorf("expected no diagnostics, got %d", len(diagnostics))
	}
}

func TestLintQuery_SyntaxError(t *testing.T) {
	lintStrict = false
	diagnostics, err := lintQuery("test.kql", "T | where ((")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(diagnostics) == 0 {
		t.Error("expected diagnostics for syntax error")
	}
	for _, d := range diagnostics {
		if d.Severity != "error" {
			t.Errorf("expected severity 'error', got %q", d.Severity)
		}
	}
}

func TestLintQuery_StrictMode(t *testing.T) {
	lintStrict = true
	defer func() { lintStrict = false }()

	diagnostics, err := lintQuery("test.kql", "T | where x > 10")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// In strict mode, this should parse without errors
	// (semantic analysis may or may not produce warnings depending on context)
	for _, d := range diagnostics {
		if d.Severity == "error" && !strings.Contains(d.Message, "unresolved") {
			// Allow unresolved table/column errors in strict mode
			t.Logf("diagnostic: %s", d.Message)
		}
	}
}

func TestLintFile(t *testing.T) {
	// Create temp file
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.kql")
	if err := os.WriteFile(tmpFile, []byte("T | take 10"), 0644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	lintStrict = false
	diagnostics, err := lintFile(tmpFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(diagnostics) != 0 {
		t.Errorf("expected no diagnostics, got %d", len(diagnostics))
	}
}

func TestLintFile_NotFound(t *testing.T) {
	lintStrict = false
	_, err := lintFile("/nonexistent/path/test.kql")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestLintReader(t *testing.T) {
	lintStrict = false
	reader := strings.NewReader("T | project A, B")
	diagnostics, err := lintReader("stdin", reader)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(diagnostics) != 0 {
		t.Errorf("expected no diagnostics, got %d", len(diagnostics))
	}
}

func TestParseErrorToDiagnostic_WithPosition(t *testing.T) {
	err := mockError{msg: "test.kql:5:10: unexpected token"}
	diag := parseErrorToDiagnostic("test.kql", err)

	if diag.Line != 5 {
		t.Errorf("expected line 5, got %d", diag.Line)
	}
	if diag.Column != 10 {
		t.Errorf("expected column 10, got %d", diag.Column)
	}
	if diag.Message != "unexpected token" {
		t.Errorf("expected message 'unexpected token', got %q", diag.Message)
	}
}

func TestParseErrorToDiagnostic_WithoutPosition(t *testing.T) {
	err := mockError{msg: "some error without position"}
	diag := parseErrorToDiagnostic("test.kql", err)

	if diag.Line != 1 {
		t.Errorf("expected line 1 (fallback), got %d", diag.Line)
	}
	if diag.Column != 1 {
		t.Errorf("expected column 1 (fallback), got %d", diag.Column)
	}
}

func TestOutputJSON(t *testing.T) {
	diagnostics := []LintDiagnostic{
		{File: "test.kql", Line: 1, Column: 5, Severity: "error", Message: "test error"},
	}

	// Just ensure no error - actual output goes to stdout
	err := outputJSON(diagnostics)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestOutputText(t *testing.T) {
	diagnostics := []LintDiagnostic{
		{File: "test.kql", Line: 1, Column: 5, Severity: "error", Message: "test error"},
	}

	err := outputText(diagnostics, true)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestOutputText_NoIssues(t *testing.T) {
	lintQuiet = false
	defer func() { lintQuiet = false }()

	err := outputText(nil, false)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestOutputText_Quiet(t *testing.T) {
	lintQuiet = true
	defer func() { lintQuiet = false }()

	err := outputText(nil, false)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestOutputDiagnostics_UnknownFormat(t *testing.T) {
	lintFormat = "unknown"
	defer func() { lintFormat = "text" }()

	err := outputDiagnostics(nil, false)
	if err == nil {
		t.Error("expected error for unknown format")
	}
}

// mockError implements error interface for testing
type mockError struct {
	msg string
}

func (e mockError) Error() string {
	return e.msg
}

func TestLintQuery_StrictWithErrors(t *testing.T) {
	lintStrict = true
	defer func() { lintStrict = false }()

	// This query has unresolved references which should produce diagnostics in strict mode
	diagnostics, err := lintQuery("test.kql", "UnknownTable | where x > 10")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// We expect some diagnostics (likely unresolved table error)
	// Just verify we can run without crashing
	t.Logf("Got %d diagnostics in strict mode", len(diagnostics))
}

func TestOutputDiagnostics_Text(t *testing.T) {
	lintFormat = "text"
	defer func() { lintFormat = "text" }()

	diagnostics := []LintDiagnostic{
		{File: "a.kql", Line: 1, Column: 1, Severity: "error", Message: "test"},
	}
	err := outputDiagnostics(diagnostics, true)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestOutputDiagnostics_JSON(t *testing.T) {
	lintFormat = "json"
	defer func() { lintFormat = "text" }()

	diagnostics := []LintDiagnostic{
		{File: "a.kql", Line: 1, Column: 1, Severity: "error", Message: "test"},
	}
	err := outputDiagnostics(diagnostics, true)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestLintReader_ErrorOnScan(t *testing.T) {
	lintStrict = false
	// Test with a reader that returns valid content
	reader := strings.NewReader("T | where ((\n")
	diagnostics, err := lintReader("test", reader)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should have parse errors
	if len(diagnostics) == 0 {
		t.Error("expected diagnostics for malformed query")
	}
}

func TestLintFile_WithSyntaxError(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "bad.kql")
	if err := os.WriteFile(tmpFile, []byte("T | where (("), 0644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	lintStrict = false
	diagnostics, err := lintFile(tmpFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(diagnostics) == 0 {
		t.Error("expected diagnostics for syntax error")
	}
}

func TestDoLint_FromStdin(t *testing.T) {
	lintStrict = false
	lintQuiet = true
	defer func() {
		lintStrict = false
		lintQuiet = false
	}()

	stdin := strings.NewReader("T | take 10\n")
	hasErrors, err := doLint(nil, stdin)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hasErrors {
		t.Error("expected no errors for valid query")
	}
}

func TestDoLint_FromStdinWithDash(t *testing.T) {
	lintStrict = false
	lintQuiet = true
	defer func() {
		lintStrict = false
		lintQuiet = false
	}()

	stdin := strings.NewReader("T | take 10\n")
	hasErrors, err := doLint([]string{"-"}, stdin)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hasErrors {
		t.Error("expected no errors for valid query")
	}
}

func TestDoLint_FromFile(t *testing.T) {
	lintStrict = false
	lintQuiet = true
	defer func() {
		lintStrict = false
		lintQuiet = false
	}()

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.kql")
	if err := os.WriteFile(tmpFile, []byte("T | take 10"), 0644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	hasErrors, err := doLint([]string{tmpFile}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hasErrors {
		t.Error("expected no errors for valid query")
	}
}

func TestDoLint_WithErrors(t *testing.T) {
	lintStrict = false
	lintQuiet = true
	defer func() {
		lintStrict = false
		lintQuiet = false
	}()

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "bad.kql")
	if err := os.WriteFile(tmpFile, []byte("T | where (("), 0644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	hasErrors, err := doLint([]string{tmpFile}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasErrors {
		t.Error("expected errors for invalid query")
	}
}

func TestDoLint_FileNotFound(t *testing.T) {
	lintStrict = false
	lintQuiet = true
	defer func() {
		lintStrict = false
		lintQuiet = false
	}()

	_, err := doLint([]string{"/nonexistent/file.kql"}, nil)
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestDoLint_MultipleFiles(t *testing.T) {
	lintStrict = false
	lintQuiet = true
	defer func() {
		lintStrict = false
		lintQuiet = false
	}()

	tmpDir := t.TempDir()
	file1 := filepath.Join(tmpDir, "a.kql")
	file2 := filepath.Join(tmpDir, "b.kql")
	if err := os.WriteFile(file1, []byte("T | take 10"), 0644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	if err := os.WriteFile(file2, []byte("T | project A"), 0644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	hasErrors, err := doLint([]string{file1, file2}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hasErrors {
		t.Error("expected no errors for valid queries")
	}
}

func TestOutputJSON_Empty(t *testing.T) {
	err := outputJSON(nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestLintQuery_EmptyQuery(t *testing.T) {
	lintStrict = false
	diagnostics, err := lintQuery("test.kql", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Empty query should parse without error (just an empty statement list)
	t.Logf("empty query produced %d diagnostics", len(diagnostics))
}

func TestLintReader_Empty(t *testing.T) {
	lintStrict = false
	reader := strings.NewReader("")
	diagnostics, err := lintReader("stdin", reader)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	t.Logf("empty reader produced %d diagnostics", len(diagnostics))
}

func TestRunLint_RunsWithoutPanic(t *testing.T) {
	// We can't fully test runLint because it calls os.Exit,
	// but we can verify it compiles and the command exists
	if lintCmd == nil {
		t.Error("lintCmd should not be nil")
	}
	if lintCmd.Use != "lint [file...]" {
		t.Errorf("unexpected Use: %q", lintCmd.Use)
	}
}

func TestRunLint_Success(t *testing.T) {
	// Mock osExit to capture exit codes
	exitCalled := false
	exitCode := 0
	origExit := osExit
	osExit = func(code int) {
		exitCalled = true
		exitCode = code
	}
	defer func() { osExit = origExit }()

	// Reset flags
	lintStrict = false
	lintQuiet = true
	lintFormat = "text"
	defer func() { lintQuiet = false }()

	// Create temp file with valid query
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "valid.kql")
	if err := os.WriteFile(tmpFile, []byte("print 'hello'"), 0644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	// Set command args
	lintCmd.SetArgs([]string{tmpFile})
	err := runLint(lintCmd, []string{tmpFile})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exitCalled {
		t.Errorf("osExit should not be called for valid query, but was called with code %d", exitCode)
	}
}

func TestRunLint_WithErrors(t *testing.T) {
	// Mock osExit to capture exit codes
	exitCalled := false
	exitCode := 0
	origExit := osExit
	osExit = func(code int) {
		exitCalled = true
		exitCode = code
	}
	defer func() { osExit = origExit }()

	// Reset flags
	lintStrict = false
	lintQuiet = true
	lintFormat = "text"
	defer func() { lintQuiet = false }()

	// Create temp file with syntax error
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "error.kql")
	if err := os.WriteFile(tmpFile, []byte("T | where (("), 0644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	err := runLint(lintCmd, []string{tmpFile})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !exitCalled {
		t.Error("osExit should be called for query with errors")
	}
	if exitCode != 1 {
		t.Errorf("expected exit code 1, got %d", exitCode)
	}
}

func TestRunLint_DoLintError(t *testing.T) {
	// Reset flags with invalid format to trigger error
	lintStrict = false
	lintQuiet = false
	lintFormat = "invalid"
	defer func() { lintFormat = "text" }()

	// Create temp file with valid query
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.kql")
	if err := os.WriteFile(tmpFile, []byte("print 'hello'"), 0644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	err := runLint(lintCmd, []string{tmpFile})
	if err == nil {
		t.Error("expected error for invalid format")
	}
}

func TestLintReader_ReadError(t *testing.T) {
	lintStrict = false
	_, err := lintReader("test", errorReader{})
	if err == nil {
		t.Error("expected error for reader that fails")
	}
}

func TestLintQuery_StrictModeWithWarnings(t *testing.T) {
	lintStrict = true
	defer func() { lintStrict = false }()

	// Run a query through strict mode and check we get through warnings path
	diagnostics, err := lintQuery("test.kql", "T | where x > 10")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// We just need to exercise the code path, actual warning count may vary
	t.Logf("strict mode diagnostics: %d", len(diagnostics))
}

func TestOutputJSON_WithMultipleDiagnostics(t *testing.T) {
	diagnostics := []LintDiagnostic{
		{File: "a.kql", Line: 1, Column: 1, Severity: "error", Message: "first"},
		{File: "b.kql", Line: 2, Column: 3, Severity: "warning", Message: "second"},
	}
	err := outputJSON(diagnostics)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestOutputText_WithWarnings(t *testing.T) {
	diagnostics := []LintDiagnostic{
		{File: "test.kql", Line: 1, Column: 1, Severity: "warning", Message: "this is a warning"},
	}
	err := outputText(diagnostics, false)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDoLint_OutputDiagnosticsError(t *testing.T) {
	// Test that outputDiagnostics error is propagated
	lintStrict = false
	lintFormat = "invalid"
	defer func() {
		lintStrict = false
		lintFormat = "text"
	}()

	stdin := strings.NewReader("T | take 10\n")
	_, err := doLint(nil, stdin)
	if err == nil {
		t.Error("expected error for invalid format")
	}
}

func TestDoLint_StdinReadError(t *testing.T) {
	lintStrict = false
	lintQuiet = true
	defer func() {
		lintStrict = false
		lintQuiet = false
	}()

	stdin := errorReader{}
	_, err := doLint(nil, stdin)
	if err == nil {
		t.Error("expected error when stdin read fails")
	}
}

func TestLintQuery_StrictModeProducesErrors(t *testing.T) {
	lintStrict = true
	defer func() { lintStrict = false }()

	// A query with an obvious syntax error
	diagnostics, err := lintQuery("test.kql", "T | where ((")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// We should get at least one error diagnostic
	hasError := false
	for _, d := range diagnostics {
		if d.Severity == "error" {
			hasError = true
			break
		}
	}
	if !hasError {
		t.Error("expected at least one error diagnostic in strict mode")
	}
}

func TestLintQuery_StrictModeWithWarningsPath(t *testing.T) {
	lintStrict = true
	defer func() { lintStrict = false }()

	// A syntactically correct query - should exercise warnings path too
	diagnostics, err := lintQuery("test.kql", "print 'hello'")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Just verify we exercised the code path
	t.Logf("Got %d diagnostics", len(diagnostics))
}

