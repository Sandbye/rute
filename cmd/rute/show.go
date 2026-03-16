package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/sandbye/rute/internal/parser"
	"github.com/sandbye/rute/internal/renderer"
	ruteYaml "github.com/sandbye/rute/internal/yaml"
	"github.com/spf13/cobra"
)

var showCmd = &cobra.Command{
	Use:   "show <path>",
	Short: "Show full detail for one endpoint",
	Args:  cobra.ExactArgs(1),
	RunE:  runShow,
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		cfg, err := loadConfig()
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		var paths []string
		for _, ep := range cfg.Endpoints {
			if toComplete == "" || strings.HasPrefix(ep.Path, toComplete) {
				paths = append(paths, ep.Path+"\t"+ep.Method+" "+ep.Description)
			}
		}
		return paths, cobra.ShellCompDirectiveNoFileComp
	},
}

func init() {
	rootCmd.AddCommand(showCmd)
}

func runShow(cmd *cobra.Command, args []string) error {
	target := args[0]

	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	var found *ruteYaml.Endpoint
	for i := range cfg.Endpoints {
		if cfg.Endpoints[i].Path == target {
			found = &cfg.Endpoints[i]
			break
		}
	}

	if found == nil {
		return fmt.Errorf("no endpoint found for path: %s", target)
	}

	baseDir := "."
	if cfgFile != "" {
		baseDir = filepath.Dir(cfgFile)
	}

	headingStyle := lipgloss.NewStyle().Bold(true)
	sectionStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true)

	// Header
	fmt.Printf("%s %s\n", renderer.MethodStyle(found.Method), headingStyle.Render(found.Path))
	if found.Description != "" {
		fmt.Println(found.Description)
	}
	fmt.Println()

	// Params
	if found.Params != nil {
		fmt.Println(sectionStyle.Render("Params"))
		renderSchemaSection(baseDir, found.Params.Schema)
	}

	// Query
	if found.Query != nil {
		fmt.Println(sectionStyle.Render("Query"))
		renderSchemaSection(baseDir, found.Query.Schema)
	}

	// Body
	if found.Body != nil {
		fmt.Println(sectionStyle.Render("Body"))
		renderSchemaSection(baseDir, found.Body.Schema)
	}

	// Response
	if len(found.Response) > 0 {
		for code, holder := range found.Response {
			fmt.Println(sectionStyle.Render(fmt.Sprintf("Response %s", code)))
			renderSchemaSection(baseDir, holder.Schema)
		}
	}

	// Examples
	if len(found.Examples) > 0 {
		for _, ex := range found.Examples {
			label := ex.Lang
			if ex.Label != "" {
				label = ex.Label
			}
			fmt.Println(sectionStyle.Render(fmt.Sprintf("Example — %s", label)))
			fmt.Println(strings.TrimRight(ex.Code, "\n"))
			fmt.Println()
		}
	}

	return nil
}

func renderSchemaSection(baseDir, ref string) {
	parsed, err := resolveAndParse(baseDir, ref)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  warning: could not parse schema %s: %v\n", ref, err)
		fmt.Println()
		return
	}
	fmt.Print(renderer.RenderSchema(parsed, "", true))
	fmt.Println()
}

func resolveAndParse(baseDir, ref string) (*parser.Schema, error) {
	sr, err := ruteYaml.ParseSchemaRef(ref)
	if err != nil {
		return nil, err
	}
	return parser.Parse(baseDir, sr.File, sr.Export)
}
