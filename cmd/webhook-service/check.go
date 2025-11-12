package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/example/gitea-jenkins-webhook/internal/config"
	"github.com/example/gitea-jenkins-webhook/internal/gitea"
	"github.com/example/gitea-jenkins-webhook/internal/jenkins"
)

type checkResult struct {
	passed   int
	errors   int
	warnings int
}

func checkCommand() {
	fs := flag.NewFlagSet("check", flag.ExitOnError)
	configPath := fs.String("config", "", "Path to configuration file")
	debugFlag := fs.Bool("debug", false, "Enable debug logging")
	fs.Parse(os.Args[1:])

	if *configPath == "" {
		fmt.Fprintf(os.Stderr, "ERROR: -config flag is required\n")
		os.Exit(1)
	}

	logger := setupLogger(*debugFlag)

	result := &checkResult{}

	fmt.Println("Checking configuration...")
	fmt.Println()

	// Stage 1: Check if config file exists
	if err := checkConfigFileExists(*configPath); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: Configuration file not found: %s\n", *configPath)
		os.Exit(1)
	}

	// Stage 2: Load and validate configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: Failed to load configuration: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✓ Configuration file loaded and validated")
	result.passed++

	// Stage 3: Validate server configuration
	if err := validateServerConfig(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: Server configuration invalid: %v\n", err)
		result.errors++
		os.Exit(1)
	}
	fmt.Println("✓ Server configuration is valid")
	result.passed++

	ctx := context.Background()

	// Stage 4: Check Jenkins accessibility
	jClient := jenkins.NewClient(cfg.Jenkins.BaseURL, cfg.Jenkins.Username, cfg.Jenkins.APIToken, nil, logger)
	if err := jClient.CheckAccessibility(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "✗ Jenkins is not accessible at %s: %v\n", cfg.Jenkins.BaseURL, err)
		result.errors++
		os.Exit(1)
	}
	fmt.Printf("✓ Jenkins is accessible at %s\n", cfg.Jenkins.BaseURL)
	result.passed++

	// Stage 5: Check Gitea accessibility
	gClient := gitea.NewClient(cfg.Gitea.BaseURL, cfg.Gitea.Token, nil, logger)
	if err := gClient.CheckAccessibility(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "✗ Gitea is not accessible at %s: %v\n", cfg.Gitea.BaseURL, err)
		result.errors++
		os.Exit(1)
	}
	fmt.Printf("✓ Gitea is accessible at %s\n", cfg.Gitea.BaseURL)
	result.passed++

	// Stage 6: Check Gitea repository access (optional)
	if len(cfg.Repositories) > 0 {
		firstRepo := cfg.Repositories[0]
		owner, repo, err := splitRepoName(firstRepo.Name)
		if err == nil {
			if err := gClient.GetRepository(ctx, owner, repo); err != nil {
				fmt.Println("⚠ Warning: Could not verify repository access (this is not critical)")
				result.warnings++
			} else {
				fmt.Println("✓ Gitea repository access verified")
				result.passed++
			}
		} else {
			fmt.Println("⚠ Warning: Could not verify repository access (this is not critical)")
			result.warnings++
		}
	} else {
		fmt.Println("⚠ Warning: No repositories configured, skipping repository access check")
		result.warnings++
	}

	// Stage 7: Check repositories
	fmt.Println()
	fmt.Println("Checking repositories:")
	for _, repoRule := range cfg.Repositories {
		fmt.Printf("  Repository: %s\n", repoRule.Name)
		checkRepository(ctx, repoRule, jClient, gClient, result)
	}

	// Print summary
	fmt.Println()
	fmt.Printf("Summary: %d checks passed, %d errors, %d warnings\n", result.passed, result.errors, result.warnings)

	if result.errors > 0 {
		os.Exit(1)
	}
	os.Exit(0)
}

func checkConfigFileExists(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("configuration file not found: %s", path)
	}
	return nil
}

func validateServerConfig(cfg *config.Config) error {
	if cfg.Server.ListenAddr == "" {
		return fmt.Errorf("server.listen_addr must be provided")
	}
	if cfg.Server.WebhookSecret == "" {
		return fmt.Errorf("server.webhook_secret must be provided")
	}
	if cfg.Server.WorkerPoolSize <= 0 {
		return fmt.Errorf("server.worker_pool_size must be > 0")
	}
	if cfg.Server.QueueSize <= 0 {
		return fmt.Errorf("server.queue_size must be > 0")
	}
	return nil
}

func checkRepository(ctx context.Context, repoRule config.RepositoryRule, jClient *jenkins.Client, gClient *gitea.Client, result *checkResult) {
	// 7.1: Check repository exists in Gitea
	owner, repo, err := splitRepoName(repoRule.Name)
	if err != nil {
		fmt.Printf("  ✗ Invalid repository name format: %s\n", repoRule.Name)
		result.errors++
		return
	}

	if err := gClient.GetRepository(ctx, owner, repo); err != nil {
		if strings.Contains(err.Error(), "not found") {
			fmt.Printf("  ✗ Repository %s does not exist in Gitea\n", repoRule.Name)
		} else if strings.Contains(err.Error(), "access denied") {
			fmt.Printf("  ✗ No access to repository %s in Gitea\n", repoRule.Name)
		} else {
			fmt.Printf("  ✗ Failed to check repository %s: %v\n", repoRule.Name, err)
		}
		result.errors++
		return
	}
	fmt.Printf("  ✓ Repository %s exists in Gitea\n", repoRule.Name)
	result.passed++

	// 7.2: Check job_root in Jenkins (if specified)
	if repoRule.JobRoot != "" {
		if err := jClient.CheckJobRootExists(ctx, repoRule.JobRoot); err != nil {
			if strings.Contains(err.Error(), "not found") {
				fmt.Printf("  ✗ Job root \"%s\" does not exist in Jenkins\n", repoRule.JobRoot)
			} else if strings.Contains(err.Error(), "access denied") {
				fmt.Printf("  ✗ No access to job root \"%s\" in Jenkins\n", repoRule.JobRoot)
			} else {
				fmt.Printf("  ✗ Failed to check job root \"%s\": %v\n", repoRule.JobRoot, err)
			}
			result.errors++
			return
		}
		fmt.Printf("  ✓ Job root \"%s\" exists in Jenkins\n", repoRule.JobRoot)
		result.passed++
	}

	// 7.3: Check for jobs in root
	jobs, err := jClient.GetJobs(ctx, repoRule.JobRoot)
	if err != nil {
		fmt.Printf("  ✗ Failed to get jobs from root \"%s\": %v\n", getJobRootDisplay(repoRule.JobRoot), err)
		result.errors++
		return
	}

	if len(jobs) == 0 {
		fmt.Printf("  ⚠ No jobs found in root \"%s\"\n", getJobRootDisplay(repoRule.JobRoot))
		result.warnings++
	} else {
		fmt.Printf("  ✓ Found %d job(s) in root \"%s\"\n", len(jobs), getJobRootDisplay(repoRule.JobRoot))
		result.passed++
	}

	// 7.4: Check job pattern match
	if len(jobs) > 0 {
		pattern, err := compileJobPattern(repoRule.JobPattern)
		if err != nil {
			fmt.Printf("  ✗ Invalid job pattern \"%s\": %v\n", repoRule.JobPattern, err)
			result.errors++
			return
		}

		matched := false
		for _, job := range jobs {
			if pattern.MatchString(job.Name) || pattern.MatchString(job.FullName) {
				matched = true
				break
			}
		}

		if matched {
			fmt.Printf("  ✓ Job pattern matches at least one job\n")
			result.passed++
		} else {
			fmt.Printf("  ✗ No jobs match pattern \"%s\"\n", repoRule.JobPattern)
			result.errors++
		}
	} else {
		fmt.Printf("  ⚠ Warning: Could not verify job pattern (no jobs found)\n")
		result.warnings++
	}
}

func splitRepoName(fullName string) (string, string, error) {
	parts := strings.SplitN(fullName, "/", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid repository name format: %s", fullName)
	}
	return parts[0], parts[1], nil
}

func getJobRootDisplay(jobRoot string) string {
	if jobRoot == "" {
		return "root"
	}
	return jobRoot
}

func compileJobPattern(pattern string) (*regexp.Regexp, error) {
	// Replace {{ .Number }} with \d+
	replaced := strings.ReplaceAll(pattern, "{{ .Number }}", `\d+`)
	// Escape special regex characters that might be in the pattern
	// But we need to be careful - the pattern might already contain regex syntax
	// So we'll just compile it as-is after replacing {{ .Number }}
	compiled, err := regexp.Compile(replaced)
	if err != nil {
		return nil, fmt.Errorf("invalid regex pattern: %w", err)
	}
	return compiled, nil
}
