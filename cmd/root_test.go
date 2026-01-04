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
	"testing"
)

func TestExecute(t *testing.T) {
	// Reset args to avoid interference
	rootCmd.SetArgs([]string{"--help"})

	err := Execute()
	if err != nil {
		t.Errorf("Execute() with --help failed: %v", err)
	}
}

func TestRootCmdUsage(t *testing.T) {
	if rootCmd.Use != "kql" {
		t.Errorf("expected Use to be 'kql', got %q", rootCmd.Use)
	}
}

func TestRootCmdHasSubcommands(t *testing.T) {
	commands := rootCmd.Commands()
	if len(commands) == 0 {
		t.Error("expected root command to have subcommands")
	}

	// Check for expected subcommands
	expectedCmds := map[string]bool{
		"link":    false,
		"lint":    false,
		"version": false,
	}

	for _, cmd := range commands {
		if _, ok := expectedCmds[cmd.Name()]; ok {
			expectedCmds[cmd.Name()] = true
		}
	}

	for name, found := range expectedCmds {
		if !found {
			t.Errorf("expected subcommand %q not found", name)
		}
	}
}
