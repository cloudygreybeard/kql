// Copyright 2026 cloudygreybeard
// SPDX-License-Identifier: Apache-2.0

package ai

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/cloudygreybeard/kqlparser"
)

// GenerateResult holds the result of a generation with validation.
type GenerateResult struct {
	// Query is the generated KQL query
	Query string

	// Valid indicates if the query passed validation
	Valid bool

	// Errors contains validation errors (if any)
	Errors []ValidationError

	// Attempts is the number of generation attempts made
	Attempts int
}

// ValidationError represents a single validation error.
type ValidationError struct {
	Line    int
	Column  int
	Message string
}

// GenerateRequest holds parameters for KQL generation.
type GenerateRequest struct {
	// Prompt is the user's request/description
	Prompt string

	// Table is the optional target table name
	Table string

	// Schema is the optional table schema
	Schema string
}

// GenerateWithValidation generates KQL with validation and retry logic.
func GenerateWithValidation(
	ctx context.Context,
	provider Provider,
	req GenerateRequest,
	cfg ValidationConfig,
	baseTemp float32,
	buildPrompt func(GenerateRequest) string,
	extractKQL func(string) string,
	verbose io.Writer,
	debug io.Writer,
) (*GenerateResult, error) {
	if !cfg.Enabled {
		// Validation disabled: single attempt, no validation
		prompt := buildPrompt(req)
		response, err := provider.Complete(ctx, prompt)
		if err != nil {
			return nil, fmt.Errorf("generating query: %w", err)
		}
		return &GenerateResult{
			Query:    extractKQL(response),
			Valid:    true, // Assume valid when not checking
			Attempts: 1,
		}, nil
	}

	var lastKQL string
	var lastErrors []ValidationError
	maxAttempts := cfg.Retries + 1

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		// Build prompt (with retry feedback if applicable)
		var prompt string
		if attempt == 1 {
			prompt = buildPrompt(req)
		} else {
			prompt = buildRetryPrompt(req, lastKQL, lastErrors, attempt, cfg.Feedback, buildPrompt)
		}

		// Adjust temperature on retries
		temp := baseTemp
		if attempt > 1 && cfg.Temp.Adjust {
			temp = baseTemp + (float32(attempt-1) * cfg.Temp.Increment)
			if temp > cfg.Temp.Max {
				temp = cfg.Temp.Max
			}
		}

		// Log attempt if verbose
		if verbose != nil {
			if attempt == 1 {
				fmt.Fprintf(verbose, "Attempt %d/%d: generating...\n", attempt, maxAttempts)
			} else {
				fmt.Fprintf(verbose, "Attempt %d/%d: retrying with error feedback (temp=%.2f)...\n", attempt, maxAttempts, temp)
			}
		}

		// Generate with potentially adjusted temperature
		response, err := provider.Complete(ctx, prompt)
		if err != nil {
			return nil, fmt.Errorf("generating query (attempt %d): %w", attempt, err)
		}

		// Debug: show raw response
		if debug != nil {
			fmt.Fprintf(debug, "--- Raw LLM Response (attempt %d) ---\n%s\n--- End Raw Response ---\n", attempt, response)
		}

		kql := extractKQL(response)
		lastKQL = kql

		// Debug: show extracted KQL
		if debug != nil {
			fmt.Fprintf(debug, "--- Extracted KQL ---\n%s\n--- End Extracted ---\n\n", kql)
		}

		// Validate
		parseResult := kqlparser.Parse("generated.kql", kql)
		if len(parseResult.Errors) == 0 {
			if verbose != nil {
				fmt.Fprintf(verbose, "  ✓ Valid KQL\n")
			}
			return &GenerateResult{
				Query:    kql,
				Valid:    true,
				Attempts: attempt,
			}, nil
		}

		// Convert errors (parse error message format: "file:line:col: message")
		lastErrors = make([]ValidationError, len(parseResult.Errors))
		for i, e := range parseResult.Errors {
			lastErrors[i] = parseErrorToValidationError(e)
		}

		if verbose != nil {
			fmt.Fprintf(verbose, "  ✗ %d syntax error(s)\n", len(lastErrors))
			for _, e := range lastErrors {
				fmt.Fprintf(verbose, "    Line %d, Col %d: %s\n", e.Line, e.Column, e.Message)
			}
		}
	}

	// All attempts exhausted
	return &GenerateResult{
		Query:    lastKQL,
		Valid:    false,
		Errors:   lastErrors,
		Attempts: maxAttempts,
	}, nil
}

// buildRetryPrompt builds a prompt that includes error feedback from previous attempt.
func buildRetryPrompt(
	req GenerateRequest,
	failedKQL string,
	errors []ValidationError,
	attempt int,
	feedback FeedbackConfig,
	buildPrompt func(GenerateRequest) string,
) string {
	var sb strings.Builder

	// Start with original prompt
	sb.WriteString(buildPrompt(req))
	sb.WriteString("\n\n---\n\n")
	sb.WriteString("Your previous attempt had syntax errors:\n\n```kql\n")
	sb.WriteString(failedKQL)
	sb.WriteString("\n```\n\n")

	// Include error messages
	if feedback.Errors {
		sb.WriteString("Errors:\n")
		for _, e := range errors {
			fmt.Fprintf(&sb, "- Line %d, Column %d: %s\n", e.Line, e.Column, e.Message)
		}
		sb.WriteString("\n")
	}

	// Include hints for error types
	if feedback.Hints {
		hints := getErrorHints(errors)
		if len(hints) > 0 {
			sb.WriteString("Hints:\n")
			for _, h := range hints {
				fmt.Fprintf(&sb, "- %s\n", h)
			}
			sb.WriteString("\n")
		}
	}

	// Include syntax examples (more on later attempts if progressive)
	if feedback.Examples {
		examples := getErrorExamples(errors, attempt, feedback.Progressive)
		if len(examples) > 0 {
			sb.WriteString("Correct syntax examples:\n")
			for _, ex := range examples {
				fmt.Fprintf(&sb, "%s\n", ex)
			}
			sb.WriteString("\n")
		}
	}

	// Progressive: add more emphasis on later attempts
	if feedback.Progressive && attempt >= 3 {
		sb.WriteString("IMPORTANT: Please carefully check all parentheses, pipes, and operator syntax.\n\n")
	}

	sb.WriteString("Please fix these errors and provide a corrected query.")

	return sb.String()
}

// getErrorHints returns contextual hints based on error types.
func getErrorHints(errors []ValidationError) []string {
	hints := make(map[string]bool)

	for _, e := range errors {
		msg := strings.ToLower(e.Message)

		// Parenthesis issues
		if strings.Contains(msg, "expected ')'") || strings.Contains(msg, "expected '('") ||
			strings.Contains(msg, "unclosed") || strings.Contains(msg, "unmatched") {
			hints["Ensure all parentheses are balanced"] = true
		}

		// Pipe issues
		if strings.Contains(msg, "expected '|'") || strings.Contains(msg, "pipe") {
			hints["Each operator should be on a new line starting with |"] = true
		}

		// Comma issues
		if strings.Contains(msg, "expected ','") {
			hints["Multiple arguments should be separated by commas"] = true
		}

		// Operator issues
		if strings.Contains(msg, "expected operator") || strings.Contains(msg, "unknown operator") {
			hints["Common operators: where, project, summarize, extend, join, take, top, sort"] = true
		}

		// By clause issues
		if strings.Contains(msg, "by") {
			hints["The 'by' clause is used with summarize, top, and order operators"] = true
		}

		// String literal issues
		if strings.Contains(msg, "string") || strings.Contains(msg, "quote") {
			hints["Use single or double quotes for string literals"] = true
		}

		// Backtick/multi-line string issues (LLM wrapping output in backticks)
		if strings.Contains(msg, "triple delimiter") || strings.Contains(msg, "multi-line string") ||
			strings.Contains(msg, "illegal") {
			hints["Do NOT wrap output in backticks - output raw KQL only"] = true
		}

		// Datetime issues
		if strings.Contains(msg, "datetime") || strings.Contains(msg, "date") {
			hints["Use datetime() for date values, e.g., datetime(2024-01-01)"] = true
		}

		// Timespan issues
		if strings.Contains(msg, "timespan") || strings.Contains(msg, "ago") {
			hints["Use timespan literals like 1h, 7d, 30m or the ago() function"] = true
		}
	}

	result := make([]string, 0, len(hints))
	for h := range hints {
		result = append(result, h)
	}
	return result
}

// getErrorExamples returns syntax examples based on error types.
func getErrorExamples(errors []ValidationError, attempt int, progressive bool) []string {
	examples := make(map[string]bool)

	for _, e := range errors {
		msg := strings.ToLower(e.Message)

		// Summarize syntax
		if strings.Contains(msg, "summarize") || strings.Contains(msg, "count") ||
			strings.Contains(msg, "sum") || strings.Contains(msg, "avg") {
			examples["T | summarize count() by Column"] = true
			examples["T | summarize Total=sum(Value) by Category"] = true
		}

		// Where syntax
		if strings.Contains(msg, "where") || strings.Contains(msg, "filter") {
			examples["T | where Column > 10"] = true
			examples["T | where Name == 'value'"] = true
		}

		// Project syntax
		if strings.Contains(msg, "project") {
			examples["T | project Column1, Column2"] = true
			examples["T | project NewName = OldName"] = true
		}

		// Join syntax
		if strings.Contains(msg, "join") {
			examples["T1 | join kind=inner T2 on CommonColumn"] = true
		}

		// Extend syntax
		if strings.Contains(msg, "extend") {
			examples["T | extend NewColumn = Expression"] = true
		}

		// General parenthesis
		if strings.Contains(msg, "expected ')'") || strings.Contains(msg, "expected '('") {
			examples["Function calls: func(arg1, arg2)"] = true
		}

		// Progressive: add more examples on later attempts
		if progressive && attempt >= 3 {
			examples["// Multi-line query structure:\nTable\n| where Condition\n| summarize count() by Column"] = true
		}
	}

	result := make([]string, 0, len(examples))
	for ex := range examples {
		result = append(result, ex)
	}
	return result
}

// FormatValidationWarning formats validation errors for stderr output.
func FormatValidationWarning(result *GenerateResult) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "⚠ Warning: generated query has syntax errors (after %d attempt(s))\n", result.Attempts)
	for _, e := range result.Errors {
		fmt.Fprintf(&sb, "  Line %d, Column %d: %s\n", e.Line, e.Column, e.Message)
	}
	return sb.String()
}

// FormatValidationError formats validation errors for strict mode.
func FormatValidationError(result *GenerateResult) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Error: failed to generate valid query after %d attempt(s)\n", result.Attempts)
	for _, e := range result.Errors {
		fmt.Fprintf(&sb, "  Line %d, Column %d: %s\n", e.Line, e.Column, e.Message)
	}
	return sb.String()
}

// parseErrorToValidationError converts a parser error to ValidationError.
// Parser errors have format: "file:line:col: message"
func parseErrorToValidationError(err error) ValidationError {
	msg := err.Error()

	// Pattern: "filename:line:col: message"
	re := regexp.MustCompile(`^[^:]+:(\d+):(\d+): (.+)$`)
	if matches := re.FindStringSubmatch(msg); len(matches) == 4 {
		line, _ := strconv.Atoi(matches[1])
		col, _ := strconv.Atoi(matches[2])
		return ValidationError{
			Line:    line,
			Column:  col,
			Message: matches[3],
		}
	}

	// Fallback: just use the whole message
	return ValidationError{
		Line:    1,
		Column:  1,
		Message: msg,
	}
}
