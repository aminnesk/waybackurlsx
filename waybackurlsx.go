package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/rix4uni/waybackurlsx/banner"
	"github.com/spf13/pflag"
	"golang.org/x/time/rate"
)

type WaybackClient struct {
	client   *http.Client
	limiter  *rate.Limiter
	mu       sync.Mutex
	lastRate time.Duration
}

type Config struct {
	searchType     string
	onlySensitive  bool
	retries        int
	silent		   bool
	version		   bool
	verbose        bool
	sensitiveRegex *regexp.Regexp
}

func NewWaybackClient() *WaybackClient {
	return &WaybackClient{
		client: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     30 * time.Second,
			},
		},
		limiter:  rate.NewLimiter(rate.Every(1*time.Second), 1), // Start with 1 request per second
		lastRate: 1 * time.Second,
	}
}

func (w *WaybackClient) Do(req *http.Request) (*http.Response, error) {
	ctx := context.Background()
	if err := w.limiter.Wait(ctx); err != nil {
		return nil, err
	}
	return w.client.Do(req)
}

func (w *WaybackClient) AdjustRate(header http.Header) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Check for rate limit headers
	if retryAfter := header.Get("Retry-After"); retryAfter != "" {
		if seconds, err := time.ParseDuration(retryAfter + "s"); err == nil {
			newRate := seconds
			if newRate > w.lastRate {
				w.limiter.SetLimit(rate.Every(newRate))
				w.lastRate = newRate
				fmt.Fprintf(os.Stderr, "[VERBOSE] Rate limit adjusted to 1 request every %v\n", newRate)
			}
			return
		}
	}

	// If we get X-RateLimit-Remaining, be conservative
	if remaining := header.Get("X-RateLimit-Remaining"); remaining != "" {
		// If we're running low on requests, slow down
		if remaining == "0" || remaining == "1" {
			newRate := 5 * time.Second
			w.limiter.SetLimit(rate.Every(newRate))
			w.lastRate = newRate
			if remaining == "0" {
				fmt.Fprintf(os.Stderr, "[VERBOSE] Rate limit exceeded, slowing down to 1 request every %v\n", newRate)
			} else {
				fmt.Fprintf(os.Stderr, "[VERBOSE] Rate limit nearly exceeded, slowing down to 1 request every %v\n", newRate)
			}
		}
	}
}

func compileSensitiveRegex() *regexp.Regexp {
	// Merged regex combining both patterns
	mergedRegex := `(?i)(?:^|/)(?:` +
		// Version Control
		`\.git(?:/|$)|\.svn(/:|$)|\.hg(?:/|$)|\.bzr(?:/|$)|` +

		// Environment & Config
		`\.env(?:\.|$)|\.env\.(?:local|dev|prod|production|staging|test)|` +
		`config\.(?:php|json|yml|yaml|xml|ini|properties|conf|cfg)|` +
		`wp-config\.php|settings\.php|configuration\.php|` +
		`database\.yml|secrets\.(?:yml|yaml|json)|` +

		// Credentials & Keys
		`(?:id_rsa|id_dsa|id_ecdsa|id_ed25519)(?:\.pub)?|` +
		`\.?(?:aws|gcp|azure)_credentials|` +
		`\.?(?:ssh|api|private)[\w\-]*\.key|` +
		`\.?keystore\.jks|client-secret\.json|` +
		`service-account.*\.json|` +

		// Certificates
		`\.(?:pem|crt|cer|p12|pfx|key)$|` +

		// Backups & Dumps - Fixed to include simple zip files
		`(?:backup|dump|db|database|export|archive|copy|old|bak|save)[\w\-\.]*\.(?:` +
		`zip|tar|tar\.gz|tgz|tar\.bz2|gz|rar|7z|bz2|` +
		`sql|sqlite|db|dump|bak|old|` +
		`json|xml|csv|txt|log` +
		`)|` +

		// Database Files
		`\.(?:sql|sqlite|db|mdb|accdb|dump)$|` +

		// Docker/K8s
		`(?:docker-compose|dockerfile|\.dockerignore)(?:\.yml|\.yaml)?|` +
		`(?:kubernetes|k8s)-config\.yml|secrets\.yaml|` +

		// Cloud Config
		`\.aws/credentials|\.boto|\.s3cfg|` +
		`gcloud/.*\.json|\.azure/credentials|` +

		// Logs with potential secrets
		`(?:error|access|debug|app|application)\.log|` +

		// Source Archives
		`(?:source|src|project|backup).*\.(?:zip|tar\.gz|tgz|rar)|` +

		// Common sensitive files
		`\.htpasswd|\.htaccess|web\.config|` +
		`shadow|passwd|master\.passwd|` +
		`\.npmrc|\.pypirc|\.netrc|\.git-credentials|` +

		// Token/Session files
		`\.token|token\.txt|session\.json|` +
		`auth.*\.(?:json|txt|key|token)|` +

		// Archive files - ADDED THIS SECTION TO CATCH SIMPLE ZIP FILES
		`[^/]*\.(?:` +
		`zip|tar|tar\.gz|tgz|tar\.bz2|gz|rar|7z|bz2|` +
		`sql|sqlite|db|dump|bak|old|` +
		`pem|key|p12|pfx|crt|cer` +
		`)$|` +

		// Second regex pattern (simplified version)
		`\.git/|\.env$|wp-config\.php$|(?:backup|dump|db|credentials|secret|passwd)[\w\-\._]*\.(?:zip|tar|tar\.gz|tgz|sql|bak|gz|json|csv|txt|pem|key|p12|pfx|log|yml|yaml|env)` +
		`)`

	compiled, err := regexp.Compile(mergedRegex)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to compile sensitive regex pattern: %v\n", err)
		return nil
	}

	return compiled
}

func buildCDXURL(domain string, searchType string) string {
	encodedDomain := url.QueryEscape(domain)

	switch searchType {
	case "domain":
		return fmt.Sprintf("http://web.archive.org/cdx/search/cdx?url=%s/*&output=text&fl=timestamp,original&collapse=urlkey", encodedDomain)
	case "wildcard":
		fallthrough
	default:
		return fmt.Sprintf("http://web.archive.org/cdx/search/cdx?url=*.%s/*&output=text&fl=timestamp,original&collapse=urlkey", encodedDomain)
	}
}

func processDomainWithRetries(domain string, client *WaybackClient, config *Config) {
	domain = strings.TrimSpace(domain)
	if domain == "" {
		if config.verbose {
			fmt.Fprintf(os.Stderr, "[VERBOSE] Skipping empty domain\n")
		}
		return
	}

	// Remove protocol if present
	originalDomain := domain
	domain = strings.TrimPrefix(domain, "http://")
	domain = strings.TrimPrefix(domain, "https://")
	// Remove path if present
	domain = strings.Split(domain, "/")[0]

	if config.verbose {
		if originalDomain != domain {
			fmt.Fprintf(os.Stderr, "[VERBOSE] Processing domain: %s (normalized from: %s)\n", domain, originalDomain)
		} else {
			fmt.Fprintf(os.Stderr, "[VERBOSE] Processing domain: %s\n", domain)
		}
	}

	cdxURL := buildCDXURL(domain, config.searchType)
	
	if config.verbose {
		fmt.Fprintf(os.Stderr, "[VERBOSE] CDX API URL: %s\n", cdxURL)
	}

	var resp *http.Response
	var body []byte

	for attempt := 1; attempt <= config.retries; attempt++ {
		req, err := http.NewRequest("GET", cdxURL, nil)
		if err != nil {
			if config.verbose {
				fmt.Fprintf(os.Stderr, "[ERROR] Failed to create request for %s: %v\n", domain, err)
			}
			return
		}

		req.Header.Set("User-Agent", "WaybackURLsX/1.0")

		if config.verbose {
			fmt.Fprintf(os.Stderr, "[VERBOSE] Sending request to Wayback Machine CDX API (attempt %d/%d)\n", attempt, config.retries)
		}

		resp, err = client.Do(req)
		if err != nil {
			if config.verbose {
				fmt.Fprintf(os.Stderr, "[ERROR] Request failed for %s (attempt %d/%d): %v\n", domain, attempt, config.retries, err)
			}
			
			if attempt < config.retries {
				// Exponential backoff: wait longer between retries
				backoffTime := time.Duration(attempt*attempt) * time.Second
				if config.verbose {
					fmt.Fprintf(os.Stderr, "[VERBOSE] Waiting %v before retry...\n", backoffTime)
				}
				time.Sleep(backoffTime)
				continue
			} else {
				if config.verbose {
					fmt.Fprintf(os.Stderr, "[ERROR] All %d attempts failed for %s. Giving up.\n", config.retries, domain)
				}
				return
			}
		}
		defer resp.Body.Close()

		// Adjust rate based on response headers
		client.AdjustRate(resp.Header)

		if config.verbose {
			fmt.Fprintf(os.Stderr, "[VERBOSE] Response status: %d, Content-Length: %s\n", 
				resp.StatusCode, resp.Header.Get("Content-Length"))
		}

		if resp.StatusCode != 200 {
			if config.verbose {
				fmt.Fprintf(os.Stderr, "[ERROR] Non-200 status code for %s: %d\n", domain, resp.StatusCode)
			}
			
			if attempt < config.retries && (resp.StatusCode >= 500 || resp.StatusCode == 429) {
				// Retry on server errors and rate limits
				backoffTime := time.Duration(attempt*attempt) * time.Second
				if config.verbose {
					fmt.Fprintf(os.Stderr, "[VERBOSE] Server error/rate limit, waiting %v before retry...\n", backoffTime)
				}
				time.Sleep(backoffTime)
				continue
			} else {
				if config.verbose {
					fmt.Fprintf(os.Stderr, "[ERROR] Non-retryable status code %d for %s after %d attempts\n", resp.StatusCode, domain, attempt)
				}
				return
			}
		}

		body, err = io.ReadAll(resp.Body)
		if err != nil {
			if config.verbose {
				fmt.Fprintf(os.Stderr, "[ERROR] Failed to read response body for %s: %v\n", domain, err)
			}
			
			if attempt < config.retries {
				backoffTime := time.Duration(attempt*attempt) * time.Second
				if config.verbose {
					fmt.Fprintf(os.Stderr, "[VERBOSE] Body read error, waiting %v before retry...\n", backoffTime)
				}
				time.Sleep(backoffTime)
				continue
			} else {
				return
			}
		}

		// If we get here, the request was successful
		break
	}

	if config.verbose {
		fmt.Fprintf(os.Stderr, "[VERBOSE] Received %d bytes of data\n", len(body))
	}

	lines := strings.Split(string(body), "\n")
	
	if config.verbose {
		fmt.Fprintf(os.Stderr, "[VERBOSE] Found %d lines in response\n", len(lines))
	}

	sensitiveCount := 0
	totalCount := 0

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) != 2 {
			if config.verbose && line != "" {
				fmt.Fprintf(os.Stderr, "[VERBOSE] Skipping malformed line: %s\n", line)
			}
			continue
		}

		timestamp := parts[0]
		originalURL := parts[1]
		totalCount++

		// Build the Wayback URL
		waybackURL := fmt.Sprintf("https://web.archive.org/web/%s/%s", timestamp, originalURL)

		// If --only-sensitive flag is set, check if URL matches sensitive patterns
		if config.onlySensitive {
			if config.sensitiveRegex != nil && config.sensitiveRegex.MatchString(originalURL) {
				sensitiveCount++
				if config.verbose {
					fmt.Fprintf(os.Stderr, "[VERBOSE] Sensitive match: %s\n", originalURL)
				}
				fmt.Println(waybackURL)
			} else if config.verbose {
				fmt.Fprintf(os.Stderr, "[VERBOSE] Not sensitive: %s\n", originalURL)
			}
		} else {
			// Print all URLs if flag is not set
			fmt.Println(waybackURL)
		}
	}

	if config.verbose {
		if config.onlySensitive {
			fmt.Fprintf(os.Stderr, "[VERBOSE] Domain %s: %d sensitive URLs found out of %d total URLs\n", 
				domain, sensitiveCount, totalCount)
		} else {
			fmt.Fprintf(os.Stderr, "[VERBOSE] Domain %s: %d URLs processed\n", domain, totalCount)
		}
	}
}

func parseFlags() *Config {
	config := &Config{}

	pflag.StringVarP(&config.searchType, "type", "t", "wildcard", "Search type: wildcard (subdomains) or domain (exact domain)")
	pflag.BoolVarP(&config.onlySensitive, "only-sensitive", "s", false, "Only show URLs matching sensitive file patterns")
	pflag.IntVarP(&config.retries, "retries", "r", 1000, "Number of retries for failed requests")
	pflag.BoolVar(&config.silent, "silent", false, "Silent mode.")
	pflag.BoolVar(&config.version, "version", false, "Print the version of the tool and exit.")
	pflag.BoolVarP(&config.verbose, "verbose", "v", false, "Show verbose output including errors and processing info")
	pflag.Parse()

    if config.version {
        banner.PrintBanner()
        banner.PrintVersion()
        os.Exit(0)
    }

    if !config.silent {
        banner.PrintBanner()
    }

	// Validate search type
	if config.searchType != "wildcard" && config.searchType != "domain" {
		fmt.Fprintf(os.Stderr, "Error: --type must be either 'wildcard' or 'domain'\n")
		os.Exit(1)
	}

	// Validate retries
	if config.retries < 1 {
		fmt.Fprintf(os.Stderr, "Error: --retries must be at least 1\n")
		os.Exit(1)
	}

	// Compile sensitive regex if the flag is set
	if config.onlySensitive {
		config.sensitiveRegex = compileSensitiveRegex()
		if config.sensitiveRegex == nil {
			fmt.Fprintf(os.Stderr, "Error: Failed to compile sensitive regex pattern\n")
			os.Exit(1)
		}
		if config.verbose {
			fmt.Fprintf(os.Stderr, "[VERBOSE] Sensitive regex pattern compiled successfully\n")
		}
	}

	return config
}

func main() {
	config := parseFlags()
	
	if config.verbose {
		fmt.Fprintf(os.Stderr, "[VERBOSE] Starting WaybackURLsX with config: type=%s, only-sensitive=%t, retries=%d\n",
			config.searchType, config.onlySensitive, config.retries)
		fmt.Fprintf(os.Stderr, "[VERBOSE] Rate limiting: 1 request per second (adaptive)\n")
	}

	client := NewWaybackClient()
	scanner := bufio.NewScanner(os.Stdin)
	domainCount := 0

	for scanner.Scan() {
		domain := scanner.Text()
		processDomainWithRetries(domain, client, config)
		domainCount++
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
	}

	if config.verbose {
		fmt.Fprintf(os.Stderr, "[VERBOSE] Processing complete. %d domains processed.\n", domainCount)
	}
}