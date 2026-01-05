# kql

A command-line toolkit for Kusto Query Language (KQL) and Azure Data Explorer.

## Overview

`kql` provides utilities for working with KQL queries and Azure Data Explorer. Current capabilities include:

- **Build** shareable deep links from KQL queries
- **Extract** queries from existing deep links
- **Lint** validate KQL queries for syntax and semantic errors
- **Explain** get AI-powered explanations of KQL queries
- **Suggest** get AI-powered optimization suggestions
- **Generate** create KQL from natural language descriptions

Deep links open directly in Azure Data Explorer with your query pre-filled, making them ideal for documentation, runbooks, and issue trackers.

The tool implements the [Microsoft Kusto deep link specification](https://learn.microsoft.com/en-us/kusto/api/rest/deeplink).

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

## Usage

### Build a deep link from a query

```bash
# From stdin
echo 'StormEvents | take 10' | kql link build -c help -d Samples

# From file
kql link build -c mycluster.westeurope -d mydb -f query.kql

# As argument (for short queries)
kql link build -c help -d Samples "print 'hello'"
```

### Extract a query from a deep link

```bash
# As argument
kql link extract "https://dataexplorer.azure.com/clusters/help/databases/Samples?query=..."

# From stdin
echo 'https://dataexplorer.azure.com/...' | kql link extract

# From file
kql link extract -f url.txt
```

### Multi-line query example

```bash
kql link build -c help -d Samples << 'EOF'
StormEvents
| where StartTime >= datetime(2007-01-01) and StartTime < datetime(2008-01-01)
| summarize count() by State
| top 10 by count_
EOF
```

Output:
```
https://dataexplorer.azure.com/clusters/help/databases/Samples?query=H4sIAAAAAAAA%2FwouyS%2FKdS1LzSsp5qpRKM9ILUpVCC5JLCoJycxNVbCzVUhJLEktycxN1TAyMDDXNTDUNTDUVEjMS0FSZYOiyAKqiKtGoSS%2FQMHQACQClowHBAAA%2F%2F%2BDCRSAigAAAA%3D%3D
```

### Lint KQL queries

Validate KQL syntax and optionally perform semantic analysis:

```bash
# Lint from stdin
echo "T | where x > 10" | kql lint

# Lint a file
kql lint query.kql

# Lint multiple files
kql lint queries/*.kql

# Enable semantic analysis (type checking, name resolution)
kql lint --strict query.kql

# JSON output for CI/CD pipelines
kql lint --format json query.kql
```

The lint command returns exit code 0 if no errors are found, and 1 if errors are detected.

### Explain KQL queries with AI

Get natural language explanations of KQL queries using AI models:

```bash
# Using local Ollama (default)
kql explain "StormEvents | summarize count() by State"

# From file
kql explain -f query.kql

# Use Vertex AI (Gemini)
kql explain --provider vertex --vertex-project my-project "T | take 10"

# Use Azure OpenAI
kql explain --provider azure --azure-endpoint https://myorg.openai.azure.com \
    --azure-deployment gpt-4o "T | take 10"

# Use local InstructLab model
kql explain --provider instructlab --model kql-expert "T | take 10"
```

Supported AI providers:
- **ollama** - Local Ollama instance (Llama, Mistral, etc.)
- **instructlab** - Local InstructLab instance (fine-tuned models)
- **vertex** - Google Vertex AI (Gemini, Claude)
- **azure** - Azure OpenAI (GPT-4, GPT-4o)

### Get optimization suggestions

Get AI-powered suggestions to improve your KQL queries:

```bash
# Get all suggestions (performance, readability, correctness)
kql suggest "T | where A > 0 | where B > 0 | project A, B"

# Focus on performance only
kql suggest --focus performance "T | join kind=inner T2 on Id"

# Focus on readability
kql suggest --focus readability -f complex_query.kql

# Focus on correctness (potential bugs)
kql suggest --focus correctness "T | where Timestamp > ago(7d)"
```

### Generate KQL from natural language

Create KQL queries from plain English descriptions:

```bash
# Simple generation
kql generate "count events by state"

# With table context (improves accuracy)
kql generate --table StormEvents "show top 10 states by damage"

# With schema hint (best results)
kql generate --table StormEvents --schema "State, StartTime, DamageProperty" \
    "find events in Texas with damage over 1 million"

# Pipe the result to lint for validation
kql generate --table T "count by category" | kql lint
```

## Commands

```
kql link build     Build a deep link from a KQL query
kql link extract   Extract the query from a deep link
kql lint           Validate KQL query syntax and semantics
kql explain        Explain a KQL query using AI
kql suggest        Get AI-powered optimization suggestions
kql generate       Generate KQL from natural language
kql version        Print version information
kql help           Help about any command
kql completion     Generate shell completion scripts
```

## Flags

### `kql link build`

| Flag | Short | Description | Required |
|------|-------|-------------|----------|
| `--cluster` | `-c` | Kusto cluster name (e.g., `mycluster.westeurope`) | Yes |
| `--database` | `-d` | Database name | Yes |
| `--base-url` | `-b` | Base URL (default: `https://dataexplorer.azure.com`) | No |
| `--file` | `-f` | Read query from file | No |

### `kql link extract`

| Flag | Short | Description | Required |
|------|-------|-------------|----------|
| `--file` | `-f` | Read URL from file | No |

### `kql lint`

| Flag | Description | Default |
|------|-------------|---------|
| `--strict` | Enable semantic analysis (type checking, name resolution) | `false` |
| `--format` | Output format: `text` or `json` | `text` |
| `--quiet` | Suppress success messages | `false` |

### `kql explain`

| Flag | Description | Default |
|------|-------------|---------|
| `--provider` | AI provider: `ollama`, `instructlab`, `vertex`, `azure` | `ollama` |
| `--model` | Model name (provider-specific) | `llama3.2` |
| `--temperature` | Creativity (0.0-1.0) | `0.2` |
| `--file` `-f` | Read query from file | - |
| `--verbose` `-v` | Show additional context | `false` |
| `--timeout` | Timeout in seconds | `60` |

### `kql suggest`

| Flag | Description | Default |
|------|-------------|---------|
| `--focus` | Suggestion focus: `performance`, `readability`, `correctness`, `all` | `all` |
| `--provider` | AI provider (same as explain) | `ollama` |
| `--model` | Model name (provider-specific) | `llama3.2` |
| `--temperature` | Creativity (0.0-1.0) | `0.3` |
| `--file` `-f` | Read query from file | - |
| `--verbose` `-v` | Show additional context | `false` |
| `--timeout` | Timeout in seconds | `60` |

### `kql generate`

| Flag | Description | Default |
|------|-------------|---------|
| `--table` `-t` | Target table name | - |
| `--schema` `-s` | Table schema (comma-separated columns) | - |
| `--provider` | AI provider (same as explain) | `ollama` |
| `--model` | Model name (provider-specific) | `llama3.2` |
| `--temperature` | Creativity (0.0-1.0) | `0.2` |
| `--file` `-f` | Read description from file | - |
| `--verbose` `-v` | Show additional context | `false` |
| `--timeout` | Timeout in seconds | `60` |

### AI Provider Flags (for `explain` and `suggest`)

| Flag | Description | Default |
|------|-------------|---------|
| `--vertex-project` | GCP project ID (Vertex AI) | - |
| `--vertex-location` | GCP location (Vertex AI) | `us-central1` |
| `--azure-endpoint` | Azure OpenAI endpoint URL | - |
| `--azure-deployment` | Azure OpenAI deployment name | - |
| `--ollama-endpoint` | Ollama endpoint URL | `http://localhost:11434` |
| `--instructlab-endpoint` | InstructLab endpoint URL | `http://localhost:8000` |

## Configuration

`kql` can be configured via a YAML file at `~/.kql/config.yaml`:

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

## How it works

1. **Compression**: The query is compressed using gzip
2. **Encoding**: The compressed data is encoded with base64
3. **URL encoding**: The base64 string is URL-encoded
4. **URL construction**: The final URL is assembled with the cluster and database

This produces shorter URLs that work within browser URI length limits, even for complex queries.

## URL Format

Generated URLs follow this format:

```
https://dataexplorer.azure.com/clusters/{cluster}/databases/{database}?query={encoded}
```

## Shell Completion

Generate shell completion scripts:

```bash
# Bash
kql completion bash > /etc/bash_completion.d/kql

# Zsh
kql completion zsh > "${fpath[1]}/_kql"

# Fish
kql completion fish > ~/.config/fish/completions/kql.fish
```

## License

Apache License 2.0 - see [LICENSE](LICENSE) for details.

## References

- [Microsoft Kusto Deep Link Documentation](https://learn.microsoft.com/en-us/kusto/api/rest/deeplink)
- [Azure Data Explorer](https://dataexplorer.azure.com/)
- [kqlparser](https://github.com/cloudygreybeard/kqlparser) - The KQL parser library used by this tool

