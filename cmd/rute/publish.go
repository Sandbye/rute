package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sandbye/rute/internal/export"
	ruteYaml "github.com/sandbye/rute/internal/yaml"
)

var publishBranch string

var publishCmd = &cobra.Command{
	Use:   "publish",
	Short: "Publish docs to GitHub Pages (gh-pages branch)",
	Long: `Exports the documentation and pushes it to the gh-pages branch
of the current Git repository. GitHub Pages will serve it automatically.

Requires: git, a GitHub remote, and push access to the repository.`,
	RunE: runPublish,
}

func init() {
	publishCmd.Flags().StringVar(&publishBranch, "branch", "gh-pages", "target branch for GitHub Pages")
	rootCmd.AddCommand(publishCmd)
}

func runPublish(cmd *cobra.Command, args []string) error {
	// Verify we're in a git repo
	if err := git("rev-parse", "--git-dir"); err != nil {
		return fmt.Errorf("not a git repository")
	}

	// Get remote URL for display
	remote := gitOutput("remote", "get-url", "origin")
	if remote == "" {
		return fmt.Errorf("no git remote 'origin' found")
	}

	// Export docs to a temp directory
	cfgPath := cfgFile
	if cfgPath == "" {
		cfgPath = "rute.yaml"
	}

	cfg, err := ruteYaml.Load(cfgPath)
	if err != nil {
		return err
	}

	baseDir := filepath.Dir(cfgPath)
	tmpDir, err := os.MkdirTemp("", "rute-publish-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	fmt.Println("Exporting docs...")
	if err := export.Generate(cfg, baseDir, tmpDir); err != nil {
		return err
	}

	// Set up the gh-pages branch in the temp dir
	fmt.Printf("Publishing to branch '%s'...\n", publishBranch)

	// Init a fresh git repo in the temp dir
	if err := gitIn(tmpDir, "init", "-b", publishBranch); err != nil {
		return fmt.Errorf("git init failed: %w", err)
	}

	if err := gitIn(tmpDir, "add", "-A"); err != nil {
		return fmt.Errorf("git add failed: %w", err)
	}

	if err := gitIn(tmpDir, "commit", "-m", "docs: publish rute documentation"); err != nil {
		return fmt.Errorf("git commit failed: %w", err)
	}

	// Force push to the gh-pages branch on origin
	target := fmt.Sprintf("%s:%s", publishBranch, publishBranch)
	if err := gitIn(tmpDir, "push", "--force", remote, target); err != nil {
		return fmt.Errorf("git push failed: %w", err)
	}

	// Derive the GitHub Pages URL
	pagesURL := derivePagesURL(remote)
	fmt.Println("Published!")
	if pagesURL != "" {
		fmt.Printf("URL: %s\n", pagesURL)
	}

	return nil
}

func git(args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run()
}

func gitOutput(args ...string) string {
	out, err := exec.Command("git", args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func gitIn(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Stdout = nil
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func derivePagesURL(remote string) string {
	// Handle SSH: git@github.com:User/Repo.git
	remote = strings.TrimSuffix(remote, ".git")
	if strings.HasPrefix(remote, "git@github.com:") {
		parts := strings.SplitN(strings.TrimPrefix(remote, "git@github.com:"), "/", 2)
		if len(parts) == 2 {
			return fmt.Sprintf("https://%s.github.io/%s", strings.ToLower(parts[0]), parts[1])
		}
	}
	// Handle HTTPS: https://github.com/User/Repo
	if strings.HasPrefix(remote, "https://github.com/") {
		parts := strings.SplitN(strings.TrimPrefix(remote, "https://github.com/"), "/", 2)
		if len(parts) == 2 {
			return fmt.Sprintf("https://%s.github.io/%s", strings.ToLower(parts[0]), parts[1])
		}
	}
	return ""
}
