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

package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/cloudygreybeard/kql/pkg/auth"
	"github.com/cloudygreybeard/kql/pkg/kusto"
	"github.com/cloudygreybeard/kqlparser"
	"github.com/spf13/cobra"
)

// defaultQueryTimeout bounds a query at the client when no server-side
// timeout is requested, so that a query cannot hang indefinitely. It matches
// the cluster's own default query timeout.
const defaultQueryTimeout = 4 * time.Minute

// timeoutMargin is added to the server timeout to bound the HTTP request, so
// that the cluster's own timeout is reported in preference to a client-side
// cancellation.
const timeoutMargin = 30 * time.Second

var (
	queryCluster      string
	queryDatabase     string
	queryFile         string
	queryFormat       string
	queryOutput       string
	queryNoHeaders    bool
	queryParams       []string
	queryTimeout      time.Duration
	queryNoTruncation bool
	queryStats        string
	queryAuth         string
	queryTenant       string
	queryResource     string
	queryDryRun       bool
	queryLint         bool
)

var queryCmd = &cobra.Command{
	Use:   "query [QUERY]",
	Short: "Run a KQL query against an Azure Data Explorer cluster",
	Long: `Query executes a KQL query against an Azure Data Explorer cluster and
prints the primary result table.

The query can be provided via:
  - Positional argument (for short queries)
  - File (-f/--file flag)
  - Standard input (pipe or redirect)

Authentication uses the Azure CLI by default. kql stores no credentials of its
own: it calls 'az account get-access-token', leaving caching, refresh and
Conditional Access enforcement to the Azure CLI.

Where the Azure CLI is unsuitable, query.auth_command in the configuration
file names an external credential helper, which kql runs directly and never
through a shell. The helper is told the audience in KQL_AUTH_RESOURCE and
writes a token on standard output. A bearer token may instead be supplied in
the KQL_TOKEN environment variable.`,
	Example: `  # Short query as an argument
  kql query -c help -d Samples "StormEvents | count"

  # From a file, as JSON
  kql query -c mycluster.westeurope -d mydb -f query.kql --format json

  # From stdin, into a spreadsheet-friendly file
  cat query.kql | kql query -c help -d Samples --format csv -o out.csv

  # Against a fully qualified cluster, with a longer server timeout
  kql query -c https://help.kusto.windows.net -d Samples --timeout 10m "StormEvents | count"

  # Show the request without sending it
  kql query -c help -d Samples --dry-run "StormEvents | count"`,
	RunE: runQuery,
	// Errors are reported once by main, rather than also by cobra. Usage is
	// unhelpful for a query that reached the cluster and was rejected.
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.AddCommand(queryCmd)

	f := queryCmd.Flags()
	f.StringVarP(&queryCluster, "cluster", "c", "", "Cluster name, alias or URL (required)")
	f.StringVarP(&queryDatabase, "database", "d", "", "Database name (required)")
	f.StringVarP(&queryFile, "file", "f", "", "Read query from file")
	f.StringVar(&queryFormat, "format", "", "Output format: table, tsv, csv, json, ndjson (default table)")
	f.StringVarP(&queryOutput, "output", "o", "", "Write results to a file instead of standard output")
	f.BoolVar(&queryNoHeaders, "no-headers", false, "Omit the header row from tabular output")
	f.StringArrayVarP(&queryParams, "param", "p", nil, "Query parameter as name=value, repeatable")
	f.DurationVar(&queryTimeout, "timeout", 0, "Server-side query timeout (default: the cluster's own)")
	f.BoolVar(&queryNoTruncation, "no-truncation", false, "Disable the cluster's result size limits")
	f.StringVar(&queryStats, "stats", "", "Print query statistics to standard error: summary or full")
	// An optional value lets --stats stay a bare flag for the common case while
	// --stats=full remains available for the complete payload.
	f.Lookup("stats").NoOptDefVal = statsSummary
	f.StringVar(&queryAuth, "auth", "", "Credential source: auto, az, exec, env (default auto)")
	f.StringVar(&queryTenant, "tenant", "", "Tenant to request a token for")
	f.StringVar(&queryResource, "resource", "", "Override the token audience (default the cluster URL)")
	f.BoolVar(&queryDryRun, "dry-run", false, "Print the resolved request without sending it")
	f.BoolVar(&queryLint, "lint", false, "Validate the query locally before sending it")
}

func runQuery(cmd *cobra.Command, args []string) error {
	query, err := getInput(args, queryFile)
	if err != nil {
		return err
	}

	cfg, err := kusto.LoadConfig()
	if err != nil {
		return err
	}

	settings, err := resolveQuerySettings(cfg)
	if err != nil {
		return err
	}

	switch queryStats {
	case statsNone, statsSummary, statsFull:
	default:
		return fmt.Errorf("invalid --stats value %q: expected %s or %s", queryStats, statsSummary, statsFull)
	}

	if queryLint {
		if err := lintBeforeSend(query); err != nil {
			return err
		}
	}

	params, err := parseParams(queryParams)
	if err != nil {
		return err
	}

	req := kusto.QueryRequest{
		Database:      settings.database,
		Query:         query,
		Parameters:    params,
		ServerTimeout: settings.serverTimeout,
		NoTruncation:  queryNoTruncation,
	}

	if queryDryRun {
		return describeRequest(cmd.OutOrStdout(), settings, req)
	}

	provider, err := credentialProvider(settings)
	if err != nil {
		return err
	}

	client := &kusto.Client{
		Endpoint: settings.endpoint,
		Provider: provider,
		Resource: queryResource,
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), settings.deadline())
	defer cancel()

	result, err := client.Query(ctx, req)
	if err != nil {
		return err
	}

	out, closeOut, err := openOutput(queryOutput, cmd.OutOrStdout())
	if err != nil {
		return err
	}
	defer closeOut()

	if err := kusto.Render(out, result, settings.format, kusto.RenderOptions{
		Headers: !queryNoHeaders,
	}); err != nil {
		return fmt.Errorf("writing results: %w", err)
	}

	reportSummary(cmd.ErrOrStderr(), result, settings.format)
	return nil
}

// querySettings holds the resolved configuration for one execution.
type querySettings struct {
	endpoint string
	database string
	format   kusto.Format
	// serverTimeout bounds execution at the cluster. Zero requests none, so
	// that the cluster's own default, and any workload group policy, stay in
	// force rather than being overridden by a value kql invented.
	serverTimeout time.Duration
	auth          auth.Source
	authCommand   []string
	authEnv       map[string]string
}

// deadline bounds the HTTP request. A client-side ceiling applies even when no
// server-side timeout is requested, so that a query cannot hang indefinitely.
func (s querySettings) deadline() time.Duration {
	if s.serverTimeout > 0 {
		return s.serverTimeout + timeoutMargin
	}
	return defaultQueryTimeout + timeoutMargin
}

// resolveQuerySettings combines command-line flags with the configuration
// file, with flags taking precedence.
func resolveQuerySettings(cfg *kusto.FileConfig) (querySettings, error) {
	var s querySettings

	cluster := firstNonEmpty(queryCluster, cfg.Query.Cluster)
	if cluster == "" {
		return s, fmt.Errorf("no cluster given: use --cluster, or set query.cluster in the configuration file")
	}
	endpoint, err := kusto.ResolveCluster(cluster, cfg.Clusters)
	if err != nil {
		return s, err
	}
	s.endpoint = endpoint

	s.database = firstNonEmpty(queryDatabase, cfg.Query.Database)
	if s.database == "" {
		return s, fmt.Errorf("no database given: use --database, or set query.database in the configuration file")
	}

	format, err := kusto.ParseFormat(firstNonEmpty(queryFormat, cfg.Query.Format, string(kusto.FormatTable)))
	if err != nil {
		return s, err
	}
	s.format = format

	s.serverTimeout, err = resolveTimeout(cfg.Query.Timeout)
	if err != nil {
		return s, err
	}

	s.auth, err = auth.ParseSource(firstNonEmpty(queryAuth, cfg.Query.Auth))
	if err != nil {
		return s, err
	}
	s.authCommand = cfg.Query.AuthCommand
	s.authEnv = cfg.Query.AuthEnv
	return s, nil
}

// resolveTimeout determines the server-side timeout from the flag, then the
// configuration file. Zero means none was requested.
func resolveTimeout(configured string) (time.Duration, error) {
	if queryTimeout > 0 {
		return queryTimeout, nil
	}
	if configured == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(configured)
	if err != nil {
		return 0, fmt.Errorf("parsing query.timeout %q: %w", configured, err)
	}
	return d, nil
}

// firstNonEmpty returns the first non-empty value.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// credentialProvider assembles the credential chain named by the resolved auth
// source.
func credentialProvider(s querySettings) (auth.Provider, error) {
	env := auth.NewEnv()
	helper := auth.NewExec(s.authCommand, s.authEnv)
	az := auth.NewAzureCLI(queryTenant)

	switch s.auth {
	case auth.SourceEnv:
		return auth.NewCache(env), nil

	case auth.SourceExec:
		if len(s.authCommand) == 0 {
			return nil, fmt.Errorf("auth source %q selected, but no query.auth_command is set in %s",
				s.auth, configPath())
		}
		return auth.NewCache(helper), nil

	case auth.SourceAzureCLI:
		return auth.NewCache(az), nil

	default:
		return auth.NewCache(auth.NewChain(env, helper, az)), nil
	}
}

// configPath names the configuration file for use in a diagnostic, falling
// back to a description when its location cannot be determined.
func configPath() string {
	path, err := kusto.ConfigPath()
	if err != nil {
		return "the configuration file"
	}
	return path
}

// parseParams converts name=value flags into query parameters.
func parseParams(values []string) (map[string]string, error) {
	if len(values) == 0 {
		return nil, nil
	}

	params := make(map[string]string, len(values))
	for _, v := range values {
		name, value, ok := strings.Cut(v, "=")
		if !ok || strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("invalid parameter %q: want name=value", v)
		}
		params[strings.TrimSpace(name)] = value
	}
	return params, nil
}

// lintBeforeSend reports syntax errors without contacting the cluster, turning
// a slow remote rejection into an immediate local one.
func lintBeforeSend(query string) error {
	result := kqlparser.Parse("query", query)
	if len(result.Errors) == 0 {
		return nil
	}

	var b strings.Builder
	b.WriteString("query failed local validation:")
	for _, err := range result.Errors {
		diag := parseErrorToDiagnostic("query", err)
		fmt.Fprintf(&b, "\n  %d:%d: %s", diag.Line, diag.Column, diag.Message)
	}
	b.WriteString("\n(the query was not sent; omit --lint to send it regardless)")
	return fmt.Errorf("%s", b.String())
}

// describeRequest reports the resolved request without sending it.
func describeRequest(w io.Writer, s querySettings, req kusto.QueryRequest) error {
	resource := queryResource
	if resource == "" {
		resource = kusto.Resource(s.endpoint)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "endpoint:  %s/v2/rest/query\n", s.endpoint)
	fmt.Fprintf(&b, "database:  %s\n", req.Database)
	fmt.Fprintf(&b, "resource:  %s\n", resource)
	fmt.Fprintf(&b, "auth:      %s\n", s.auth)
	// The helper is shown so that a command kql would run can be audited
	// before it is run.
	if s.auth != auth.SourceEnv && len(s.authCommand) > 0 {
		fmt.Fprintf(&b, "helper:    %s\n", strings.Join(s.authCommand, " "))
	}
	fmt.Fprintf(&b, "timeout:   %s\n", describeTimeout(s.serverTimeout))
	fmt.Fprintf(&b, "format:    %s\n", s.format)
	for name, value := range req.Parameters {
		fmt.Fprintf(&b, "parameter: %s=%s\n", name, value)
	}
	fmt.Fprintf(&b, "query:\n%s\n", req.Query)

	_, err := io.WriteString(w, b.String())
	return err
}

// describeTimeout renders the server-side timeout for diagnostic output.
func describeTimeout(d time.Duration) string {
	if d == 0 {
		return "cluster default"
	}
	return d.String()
}

// openOutput returns the writer for results, together with a function closing
// it when it is a file.
func openOutput(path string, stdout io.Writer) (io.Writer, func(), error) {
	if path == "" {
		return stdout, func() {}, nil
	}

	f, err := os.Create(path)
	if err != nil {
		return nil, nil, fmt.Errorf("creating %s: %w", path, err)
	}
	return f, func() { _ = f.Close() }, nil
}

// Statistics verbosity levels accepted by --stats.
const (
	statsNone    = ""
	statsSummary = "summary"
	statsFull    = "full"
)

// reportSummary writes the row count and any requested statistics to standard
// error, keeping them clear of redirected results.
func reportSummary(w io.Writer, result *kusto.Result, format kusto.Format) {
	var b strings.Builder
	// Diagnostics are advisory: a failed write to stderr is not actionable.
	defer func() { _, _ = io.WriteString(w, b.String()) }()

	if format == kusto.FormatTable {
		fmt.Fprintf(&b, "\n(%d %s)\n", len(result.Rows), plural(len(result.Rows), "row", "rows"))
	}
	if queryStats == statsNone || len(result.Stats) == 0 {
		return
	}

	if queryStats == statsSummary {
		// Fall through to the raw payload when the digest comes back empty, so
		// that an unrecognised payload shape still yields the figures.
		if metrics := kusto.Summarize(result.Stats); len(metrics) > 0 {
			width := 0
			for _, metric := range metrics {
				if len(metric.Name) > width {
					width = len(metric.Name)
				}
			}
			for _, metric := range metrics {
				fmt.Fprintf(&b, "%-*s  %s\n", width, metric.Name, metric.Value)
			}
			return
		}
	}

	for _, stat := range result.Stats {
		fmt.Fprintf(&b, "%s: %s\n", stat.Name, stat.Payload)
	}
}

// plural selects the singular or plural form for n.
func plural(n int, singular, many string) string {
	if n == 1 {
		return singular
	}
	return many
}
