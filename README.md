# kql

A command-line toolkit for Kusto Query Language (KQL) and Azure Data Explorer.

## Overview

`kql` provides utilities for working with KQL queries and Azure Data Explorer. Current capabilities include:

- **Build** shareable deep links from KQL queries
- **Extract** queries from existing deep links
- **Lint** validate KQL queries for syntax and semantic errors

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

## Commands

```
kql link build     Build a deep link from a KQL query
kql link extract   Extract the query from a deep link
kql lint           Validate KQL query syntax and semantics
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

