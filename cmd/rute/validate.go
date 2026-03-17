package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/sandbye/rute/internal/parser"
	ruteYaml "github.com/sandbye/rute/internal/yaml"
)

var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Check all schema references in rute.yaml resolve correctly",
	RunE:  runValidate,
}

func init() {
	rootCmd.AddCommand(validateCmd)
}

func runValidate(cmd *cobra.Command, args []string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	cfgPath := cfgFile
	if cfgPath == "" {
		cfgPath = "rute.yaml"
	}
	baseDir := filepath.Dir(cfgPath)

	pass := lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render("✓")
	fail := lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render("✗")

	var refs []schemaLocation
	for _, ep := range cfg.Endpoints {
		label := fmt.Sprintf("%s %s", ep.Method, ep.Path)
		if ep.Params != nil && ep.Params.Schema != "" {
			refs = append(refs, schemaLocation{ref: ep.Params.Schema, endpoint: label, section: "params"})
		}
		if ep.Query != nil && ep.Query.Schema != "" {
			refs = append(refs, schemaLocation{ref: ep.Query.Schema, endpoint: label, section: "query"})
		}
		if ep.Body != nil && ep.Body.Schema != "" {
			refs = append(refs, schemaLocation{ref: ep.Body.Schema, endpoint: label, section: "body"})
		}
		for code, holder := range ep.Response {
			if holder.Schema != "" {
				refs = append(refs, schemaLocation{ref: holder.Schema, endpoint: label, section: fmt.Sprintf("response %s", code)})
			}
		}
	}

	if len(refs) == 0 {
		fmt.Println("No schema references found.")
		return nil
	}

	errors := 0
	for _, loc := range refs {
		sr, err := ruteYaml.ParseSchemaRef(loc.ref)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  %s %s → %s [%s]: %v\n", fail, loc.endpoint, loc.ref, loc.section, err)
			errors++
			continue
		}

		absFile := filepath.Join(baseDir, sr.File)
		if _, statErr := os.Stat(absFile); statErr != nil {
			fmt.Fprintf(os.Stderr, "  %s %s → %s [%s]: file not found\n", fail, loc.endpoint, loc.ref, loc.section)
			errors++
			continue
		}

		_, parseErr := parser.Parse(baseDir, sr.File, sr.Export)
		if parseErr != nil {
			fmt.Fprintf(os.Stderr, "  %s %s → %s [%s]: %v\n", fail, loc.endpoint, loc.ref, loc.section, parseErr)
			errors++
			continue
		}

		fmt.Printf("  %s %s → %s [%s]\n", pass, loc.endpoint, loc.ref, loc.section)
	}

	fmt.Println()
	if errors > 0 {
		return fmt.Errorf("%d of %d schema references failed validation", errors, len(refs))
	}
	fmt.Printf("All %d schema references valid.\n", len(refs))
	return nil
}

type schemaLocation struct {
	ref      string
	endpoint string
	section  string
}
