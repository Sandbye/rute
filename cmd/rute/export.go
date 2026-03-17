package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/sandbye/rute/internal/export"
	ruteYaml "github.com/sandbye/rute/internal/yaml"
)

var (
	outDir    string
	watchMode bool
)

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Generate a static HTML documentation site",
	RunE:  runExport,
}

func init() {
	exportCmd.Flags().StringVar(&outDir, "out", "rute-out", "output directory")
	exportCmd.Flags().BoolVar(&watchMode, "watch", false, "watch for changes and rebuild")
	rootCmd.AddCommand(exportCmd)
}

func runExport(cmd *cobra.Command, args []string) error {
	cfgPath := cfgFile
	if cfgPath == "" {
		cfgPath = "rute.yaml"
	}

	if err := buildExport(cfgPath); err != nil {
		return err
	}

	if !watchMode {
		return nil
	}

	fmt.Println("Watching for changes... (ctrl+c to stop)")
	return watchAndRebuild(cfgPath)
}

func buildExport(cfgPath string) error {
	cfg, err := ruteYaml.Load(cfgPath)
	if err != nil {
		return err
	}

	baseDir := filepath.Dir(cfgPath)
	if err := export.Generate(cfg, baseDir, outDir); err != nil {
		return err
	}

	fmt.Printf("Exported to %s/index.html\n", outDir)
	return nil
}

// watchAndRebuild polls rute.yaml and all referenced schema files for changes.
func watchAndRebuild(cfgPath string) error {
	lastMods := make(map[string]time.Time)

	// Seed initial mod times.
	updateModTimes(cfgPath, lastMods)

	for {
		time.Sleep(500 * time.Millisecond)

		changed := false
		files := collectWatchFiles(cfgPath)
		for _, f := range files {
			info, err := os.Stat(f)
			if err != nil {
				continue
			}
			if prev, ok := lastMods[f]; !ok || info.ModTime().After(prev) {
				changed = true
				lastMods[f] = info.ModTime()
			}
		}

		if changed {
			fmt.Printf("[%s] Change detected, rebuilding...\n", time.Now().Format("15:04:05"))
			if err := buildExport(cfgPath); err != nil {
				fmt.Fprintf(os.Stderr, "Rebuild error: %v\n", err)
			}
		}
	}
}

// collectWatchFiles returns the config file plus all referenced schema files.
func collectWatchFiles(cfgPath string) []string {
	files := []string{cfgPath}

	cfg, err := ruteYaml.Load(cfgPath)
	if err != nil {
		return files
	}

	baseDir := filepath.Dir(cfgPath)
	seen := make(map[string]bool)

	for _, ep := range cfg.Endpoints {
		refs := collectRefs(ep)
		for _, ref := range refs {
			sr, err := ruteYaml.ParseSchemaRef(ref)
			if err != nil {
				continue
			}
			absFile := filepath.Join(baseDir, sr.File)
			if !seen[absFile] {
				seen[absFile] = true
				files = append(files, absFile)
			}
		}
	}

	return files
}

func collectRefs(ep ruteYaml.Endpoint) []string {
	var refs []string
	if ep.Params != nil && ep.Params.Schema != "" {
		refs = append(refs, ep.Params.Schema)
	}
	if ep.Query != nil && ep.Query.Schema != "" {
		refs = append(refs, ep.Query.Schema)
	}
	if ep.Body != nil && ep.Body.Schema != "" {
		refs = append(refs, ep.Body.Schema)
	}
	for _, h := range ep.Response {
		if h.Schema != "" {
			refs = append(refs, h.Schema)
		}
	}
	return refs
}

func updateModTimes(cfgPath string, mods map[string]time.Time) {
	for _, f := range collectWatchFiles(cfgPath) {
		if info, err := os.Stat(f); err == nil {
			mods[f] = info.ModTime()
		}
	}
}
