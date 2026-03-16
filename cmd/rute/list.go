package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/sandbye/rute/internal/renderer"
	"github.com/sandbye/rute/internal/yaml"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all endpoints",
	RunE:  runList,
}

func init() {
	rootCmd.AddCommand(listCmd)
}

func runList(cmd *cobra.Command, args []string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	pathStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

	// Determine column widths.
	maxMethod := 6
	maxPath := 0
	for _, ep := range cfg.Endpoints {
		if len(ep.Path) > maxPath {
			maxPath = len(ep.Path)
		}
	}

	for _, ep := range cfg.Endpoints {
		method := renderer.MethodStyle(ep.Method)
		// Pad method to align paths (ANSI escape codes don't count toward width).
		padding := strings.Repeat(" ", maxMethod-len(ep.Method)+2)
		path := pathStyle.Render(ep.Path)
		pathPad := strings.Repeat(" ", maxPath-len(ep.Path)+2)
		desc := dimStyle.Render(ep.Description)
		fmt.Printf("%s%s%s%s%s\n", method, padding, path, pathPad, desc)
	}

	return nil
}

func loadConfig() (*yaml.Config, error) {
	path := cfgFile
	if path == "" {
		path = "rute.yaml"
	}
	return yaml.Load(path)
}
