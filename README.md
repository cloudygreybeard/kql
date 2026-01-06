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
| `vertex` | Google Vertex AI (Claude, Gemini) | GCP project with Vertex API + Model Garden |
| `azure` | Azure OpenAI (GPT-4, GPT-4o) | Azure OpenAI deployment |

### Explain

Get natural language explanations of KQL queries:

```bash
# Local Ollama (default)
kql explain "StormEvents | summarize count() by State"

# From file
kql explain -f query.kql

# Vertex AI with Claude (recommended for best quality)
kql explain --provider vertex --vertex-project my-project \
    --model claude-opus-4-5 "T | take 10"

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
# Simple generation (with validation, default)
kql generate "count events by state"

# With table context
kql generate --table StormEvents "show top 10 states by damage"

# With schema hint
kql generate --table StormEvents --schema "State, StartTime, DamageProperty" \
    "find events in Texas with damage over 1 million"

# Strict mode: fail if AI can't generate valid KQL
kql generate --strict "summarize by category"

# Skip validation for raw model output
kql generate --no-validate "complex request"

# Use preset for quick configuration
kql generate --preset thorough "count by state"  # More retries
kql generate --preset minimal "count by state"   # No retries, faster
```

### Fix

Get AI-suggested fixes for syntax errors:

```bash
# Fix a broken query (with validation and retries)
kql fix "T | summarize count( by State"

# Preview without output
kql fix --dry-run "T | where x >"

# Verbose (show errors, attempts, and reasoning)
kql fix -v "T | summarize count( by State"

# Strict mode: fail if fix still has errors
kql fix --strict "T | where x >"

# Custom retry count
kql fix --retries 5 "complex broken query"

# Fix and save
kql fix -f broken.kql > fixed.kql
```

### Output Validation

The `generate` and `fix` commands validate AI-generated KQL before output:

1. **Parse** the generated query with `kqlparser`
2. **Retry** with error feedback if validation fails (default: 2 retries)
3. **Output** with warning (default) or fail (strict mode)

Retry prompts include:
- Error messages from the parser
- Contextual hints for common mistakes
- Syntax examples for relevant operators
- Progressive detail on subsequent attempts

```bash
# Verbose mode shows the validation process
$ kql generate -v "count by state"
Using ollama provider with model llama3.2...
Validation: enabled (retries=2, strict=false)
Attempt 1/3: generating...
  ✗ 1 syntax error(s)
    Line 1, Col 15: expected ')' before 'by'
Attempt 2/3: retrying with error feedback (temp=0.30)...
  ✓ Valid KQL
StormEvents | summarize count() by State
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

  # Vertex AI with Claude (requires Model Garden access)
  vertex:
    project: my-gcp-project
    location: us-east5  # us-east5 for Claude, us-central1 for Gemini

  azure:
    endpoint: https://myorg.openai.azure.com
    deployment: gpt-4o-deployment

  instructlab:
    endpoint: http://localhost:8000

  # Validation settings for generate and fix commands
  validation:
    enabled: true
    strict: false
    retries: 2
    feedback:
      errors: true
      hints: true
      examples: true
      progressive: true
    temperature:
      adjust: true
      increment: 0.1
      max: 0.8
```

Command-line flags override configuration file settings. Environment variables can also be used:

| Variable | Description |
|----------|-------------|
| `KQL_VALIDATE` | Enable/disable validation (`true`/`false`) |
| `KQL_VALIDATE_STRICT` | Enable strict mode |

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
| `--vertex-location` | GCP region | `us-east5` |
| `--azure-endpoint` | Azure OpenAI endpoint | - |
| `--azure-deployment` | Azure OpenAI deployment | - |

### Validation Flags (`generate`, `fix`)

| Flag | Description | Default |
|------|-------------|---------|
| `--no-validate` | Disable validation | `false` |
| `--strict` | Fail with exit code 1 if invalid | `false` |
| `--retries` | Retry count on failure | `2` |
| `--preset` | Configuration preset | - |
| `--no-feedback` | Disable all feedback strategies | `false` |
| `--no-feedback-errors` | Disable error feedback | `false` |
| `--no-feedback-hints` | Disable hints | `false` |
| `--no-feedback-examples` | Disable examples | `false` |
| `--no-feedback-progressive` | Disable progressive detail | `false` |
| `--no-retry-temp-adjust` | Disable temperature adjustment | `false` |
| `--retry-temp-increment` | Temperature increment per retry | `0.1` |
| `--retry-temp-max` | Max temperature on retry | `0.8` |

**Presets:**

| Preset | Description |
|--------|-------------|
| `minimal` | No retries, no hints/examples (fast) |
| `balanced` | Default settings |
| `thorough` | 5 retries, progressive feedback |
| `strict` | Strict mode with 3 retries |

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
