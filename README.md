# WaybackURLsX

A powerful and efficient Go-based tool for extracting archived URLs from the Wayback Machine with advanced filtering capabilities, rate limiting, and automatic retries.

## Features
- 🚀 **High Performance**: Built in Go for fast processing
- 🎯 **Smart Filtering**: Filter results to show only sensitive files (credentials, backups, configs, etc.)
- 🔄 **Automatic Retries**: Configurable retry mechanism with exponential backoff
- ⚡ **Rate Limiting**: Adaptive rate limiting that respects Wayback Machine's limits
- 🔍 **Flexible Search**: Search by exact domain or include all subdomains
- 📊 **Verbose Mode**: Detailed logging for debugging and monitoring
- 🛡️ **Respectful Scraping**: Proper User-Agent and conservative defaults

## Rate Limiting
The tool implements adaptive rate limiting:
- Starts with 1 request per second
- Automatically adjusts based on server headers (`Retry-After`, `X-RateLimit-Remaining`)
- Slows down when approaching rate limits
- Respectful of Wayback Machine's infrastructure

## Error Handling & Retries
- **Automatic Retries**: Configurable up to 1000 attempts (default)
- **Exponential Backoff**: Wait time increases with each retry (1s, 4s, 9s, ...)
- **Smart Retry Logic**: Retries on connection errors, server errors (5xx), and rate limits (429)
- **Persistent**: Will continue retrying until successful or retry limit reached

## Installation
```
go install github.com/rix4uni/waybackurlsx@latest
```

## Download prebuilt binaries
```
wget https://github.com/rix4uni/waybackurlsx/releases/download/v0.0.1/waybackurlsx-linux-amd64-0.0.1.tgz
tar -xvzf waybackurlsx-linux-amd64-0.0.1.tgz
rm -rf waybackurlsx-linux-amd64-0.0.1.tgz
mv waybackurlsx ~/go/bin/waybackurlsx
```
Or download [binary release](https://github.com/rix4uni/waybackurlsx/releases) for your platform.

## Compile from source
```
git clone --depth 1 https://github.com/rix4uni/waybackurlsx.git
cd waybackurlsx; go install
```

## Usage
```yaml
Usage of waybackurlsx:
  -s, --only-sensitive   Only show URLs matching sensitive file patterns
  -r, --retries int      Number of retries for failed requests (default 1000)
      --silent           Silent mode.
  -t, --type string      Search type: wildcard (subdomains) or domain (exact domain) (default "wildcard")
  -v, --verbose          Show verbose output including errors and processing info
      --version          Print the version of the tool and exit.
```

## Usage Examples
### Basic Usage

```yaml
# Single domain
echo "example.com" | waybackurlsx

# Multiple domains from file
cat domains.txt | waybackurlsx

# Save output to file
echo "example.com" | waybackurlsx > urls.txt
```

### Advanced Usage

```yaml
# Only show sensitive files (credentials, backups, configs, etc.)
echo "example.com" | waybackurlsx --only-sensitive

# Search exact domain only (no subdomains)
echo "example.com" | waybackurlsx --type domain

# Verbose output with sensitive files only
echo "example.com" | waybackurlsx --only-sensitive --verbose

# Custom retry attempts for unreliable connections
echo "example.com" | waybackurlsx --retries 50 --verbose

# Combine all flags
cat domains.txt | waybackurlsx --type wildcard --only-sensitive --retries 1000 --verbose
```

## Command Line Flags

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--type` | `-t` | `wildcard` | Search type: `wildcard` (includes subdomains) or `domain` (exact domain only) |
| `--only-sensitive` | | `false` | Only show URLs matching sensitive file patterns |
| `--verbose` | `-v` | `false` | Show verbose output including errors and processing info |
| `--retries` | | `1000` | Number of retries for failed requests |

## Sensitive File Detection

When using `--only-sensitive`, the tool matches URLs against a comprehensive regex pattern that detects:

- **Version Control**: `.git/`, `.svn/`, `.hg/`, `.bzr/`
- **Environment Files**: `.env`, config files, WordPress configs
- **Credentials & Keys**: SSH keys, API keys, certificates (PEM, CRT, KEY, P12)
- **Database Files**: SQL dumps, database backups, export files
- **Backups & Archives**: ZIP, TAR, RAR files with sensitive names
- **Cloud Credentials**: AWS, GCP, Azure configuration files
- **Log Files**: Application logs that may contain secrets
- **Configuration Files**: Docker, Kubernetes, application configs

### Example Sensitive Files Matched:
```yaml
https://web.archive.org/web/20230101/http://example.com/.env
https://web.archive.org/web/20230101/http://example.com/backup.zip
https://web.archive.org/web/20230101/http://example.com/config.php
https://web.archive.org/web/20230101/http://example.com/id_rsa
https://web.archive.org/web/20230101/http://example.com/database.sql
```

### Basic Domain Enumeration
```yaml
echo "acorns.com" | waybackurlsx

Output:
https://web.archive.org/web/20210916045902/http://youngmoneyplan.acorns.com/js/youngmoney.js
https://web.archive.org/web/20180526214542/http://youngmoneyplan.acorns.com/robots.txt
```

### Sensitive Files Only
```yaml
echo "sagadb.org" | waybackurlsx --only-sensitive --verbose

Output:
[VERBOSE] Processing domain: sagadb.org
[VERBOSE] Found 2 sensitive URLs out of 1113 total URLs
https://web.archive.org/web/20150327012218/http://sagadb.org/files/zip/epub.zip
https://web.archive.org/web/20181007034211/http://sagadb.org/files/zip/sagadb_tools.zip
```

### Batch Processing with Custom Retries
```yaml
cat domains.txt | waybackurlsx --type domain --only-sensitive --retries 100 --verbose > sensitive_urls.txt
```

## Output Format
The tool outputs Wayback Machine URLs in the format:
```
https://web.archive.org/web/{timestamp}/{original_url}
```

<img width="1912" height="1032" alt="image" src="https://github.com/user-attachments/assets/da3162b7-8c82-4943-be1f-0b2e3a68a634" />
<img width="1912" height="1032" alt="image" src="https://github.com/user-attachments/assets/59082da7-000f-40ca-a0d4-ed73a40eef5c" />

