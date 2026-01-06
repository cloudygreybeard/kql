# kql

A command-line toolkit for Kusto Query Language (KQL) and Azure Data Explorer.

## Overview

`kql` is a developer toolkit for working with KQL queries. It combines practical utilities with AI-powered assistance to help you write, validate, share, and understand KQL.

### Commands

| Command | Description |
|---------|-------------|
| `kql link build` | Create shareable deep links from KQL queries |
| `kql link extract` | Extract queries from existing deep links |
| `kql lint` | Validate KQL syntax and semantics |
| `kql explain` | Get AI-powered explanations of queries |
| `kql suggest` | Get AI-powered optimization suggestions |
| `kql generate` | Create KQL from natural language |
| `kql fix` | Get AI-suggested fixes for syntax errors |

## Installation

### Homebrew (macOS/Linux)

```bash
brew install cloudygreybeard/tap/kql
```

### Go Install

```bash
go install github.com/cloudygreybeard/kql@latest
```

### From Source

```bash
git clone https://github.com/cloudygreybeard/kql.git
cd kql
make build
```

### Binary Releases

Download pre-built binaries from the [Releases page](https://github.com/cloudygreybeard/kql/releases).

## Deep Links

Deep links open directly in Azure Data Explorer with your query pre-filled—ideal for documentation, runbooks, and sharing.

### Build a deep link

```bash
# From stdin
echo 'StormEvents | take 10' | kql link build -c help -d Samples

# From file
kql link build -c mycluster.westeurope -d mydb -f query.kql

# Inline (short queries)
kql link build -c help -d Samples "print 'hello'"

# Multi-line with heredoc
kql link build -c help -d Samples << 'EOF'
StormEvents
| where StartTime >= datetime(2007-01-01) and StartTime < datetime(2008-01-01)
| summarize count() by State
| top 10 by count_
EOF
```

### Extract a query

```bash
# From argument
kql link extract "https://dataexplorer.azure.com/clusters/help/databases/Samples?query=..."

# From stdin
pbpaste | kql link extract

# From file
kql link extract -f url.txt
```

### How deep links work

1. The query is compressed with gzip
2. Compressed data is encoded as base64
3. The base64 string is URL-encoded
4. The URL is assembled with cluster and database

This follows the [Microsoft Kusto deep link specification](https://learn.microsoft.com/en-us/kusto/api/rest/deeplink) and produces compact URLs that work within browser limits.

## Validation

The `lint` command validates KQL syntax using the [kqlparser](https://github.com/cloudygreybeard/kqlparser) library.

```bash
# Validate from stdin
echo "T | where x > 10" | kql lint

# Validate a file
kql lint query.kql

# Validate multiple files
kql lint queries/*.kql

# Enable semantic analysis (type checking, name resolution)
kql lint --strict query.kql

# JSON output for CI/CD
kql lint --format json query.kql
```

Exit codes: `0` = valid, `1` = errors found.

## AI-Powered Commands

`kql` integrates with local and cloud AI models for query explanation, optimization, generation, and error correction.

### Providers

| Provider | Description | Setup |
|----------|-------------|-------|
| `ollama` | Local models (Llama, Mistral, etc.) | [Install Ollama](https://ollama.ai) |
| `instructlab` | Local fine-tuned models | [Install InstructLab](https://instructlab.ai) |
| `vertex` | Google Vertex AI (Gemini, Claude) | GCP project with Vertex API enabled |
| `azure` | Azure OpenAI (GPT-4, GPT-4o) | Azure OpenAI deployment |

### Explain

Get natural language explanations of KQL queries:

```bash
# Local Ollama (default)
kql explain "StormEvents | summarize count() by State"

# From file
kql explain -f query.kql

# Vertex AI
kql explain --provider vertex --vertex-project my-project "T | take 10"

# Azure OpenAI
kql explain --provider azure --azure-endpoint https://myorg.openai.azure.com \
    --azure-deployment gpt-4o "T | take 10"
```

### Suggest

Get optimization suggestions for performance, readability, or correctness:

```bash
# All suggestions
kql suggest "T | where A > 0 | where B > 0 | project A, B"

# Focus on performance
kql suggest --focus performance "T | join kind=inner T2 on Id"

# Focus on readability
kql suggest --focus readability -f complex_query.kql
```

### Generate

Create KQL from natural language descriptions:

```bash
# Simple generation
kql generate "count events by state"

# With table context
kql generate --table StormEvents "show top 10 states by damage"

# With schema hint
kql generate --table StormEvents --schema "State, StartTime, DamageProperty" \
    "find events in Texas with damage over 1 million"

# Validate the result
kql generate --table T "count by category" | kql lint
```

### Fix

Get AI-suggested fixes for syntax errors:

```bash
# Fix a broken query
kql fix "T | summarize count( by State"

# Preview without output
kql fix --dry-run "T | where x >"

# Verbose (show errors and reasoning)
kql fix -v "T | summarize count( by State"

# Fix and save
kql fix -f broken.kql > fixed.kql
```

## Configuration

Configure defaults in `~/.kql/config.yaml`:

```yaml
ai:
  provider: ollama
  model: llama3.2
  temperature: 0.2

  ollama:
    endpoint: http://localhost:11434

  vertex:
    project: my-gcp-project
    location: us-central1

  azure:
    endpoint: https://myorg.openai.azure.com
    deployment: gpt-4o-deployment

  instructlab:
    endpoint: http://localhost:8000
```

Command-line flags override configuration file settings.

## Flag Reference

### `kql link build`

| Flag | Short | Description | Required |
|------|-------|-------------|----------|
| `--cluster` | `-c` | Cluster name (e.g., `help`, `mycluster.westeurope`) | Yes |
| `--database` | `-d` | Database name | Yes |
| `--base-url` | `-b` | Base URL (default: `https://dataexplorer.azure.com`) | No |
| `--file` | `-f` | Read query from file | No |

### `kql link extract`

| Flag | Short | Description |
|------|-------|-------------|
| `--file` | `-f` | Read URL from file |

### `kql lint`

| Flag | Description | Default |
|------|-------------|---------|
| `--strict` | Enable semantic analysis | `false` |
| `--format` | Output format: `text`, `json` | `text` |
| `--quiet` | Suppress success messages | `false` |

### AI Commands (`explain`, `suggest`, `generate`, `fix`)

| Flag | Description | Default |
|------|-------------|---------|
| `--provider` | AI provider | `ollama` |
| `--model` | Model name | provider-specific |
| `--temperature` | Creativity (0.0–1.0) | `0.2` |
| `--file` `-f` | Read input from file | - |
| `--verbose` `-v` | Show additional context | `false` |
| `--timeout` | Timeout in seconds | `60` |

### Provider-Specific Flags

| Flag | Description | Default |
|------|-------------|---------|
| `--ollama-endpoint` | Ollama endpoint | `http://localhost:11434` |
| `--instructlab-endpoint` | InstructLab endpoint | `http://localhost:8000` |
| `--vertex-project` | GCP project ID | - |
| `--vertex-location` | GCP region | `us-central1` |
| `--azure-endpoint` | Azure OpenAI endpoint | - |
| `--azure-deployment` | Azure OpenAI deployment | - |

### `kql suggest` Additional Flags

| Flag | Description | Default |
|------|-------------|---------|
| `--focus` | Focus area: `performance`, `readability`, `correctness`, `all` | `all` |

### `kql generate` Additional Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--table` | `-t` | Target table name |
| `--schema` | `-s` | Table schema (comma-separated columns) |

### `kql fix` Additional Flags

| Flag | Description | Default |
|------|-------------|---------|
| `--dry-run` | Preview fix only | `false` |

## Shell Completion

```bash
# Bash
kql completion bash > /etc/bash_completion.d/kql

# Zsh
kql completion zsh > "${fpath[1]}/_kql"

# Fish
kql completion fish > ~/.config/fish/completions/kql.fish
```

## License

Apache License 2.0 — see [LICENSE](LICENSE).

## See Also

- [kqlparser](https://github.com/cloudygreybeard/kqlparser) — KQL parser library (used by `kql lint`)
- [Azure Data Explorer](https://dataexplorer.azure.com/)
- [KQL Reference](https://learn.microsoft.com/en-us/kusto/query/)
- [Deep Link Specification](https://learn.microsoft.com/en-us/kusto/api/rest/deeplink)
