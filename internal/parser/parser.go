// Package parser calls the Node.js extractor and returns a parsed Schema.
package parser

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/sandbye/rute/extractor"
)

// Schema represents a parsed Zod schema.
type Schema struct {
	Name   string  `json:"name"`
	Type   string  `json:"type"`
	Fields []Field `json:"fields,omitempty"`
	// For array types
	Items *Schema `json:"items,omitempty"`
	// For enum types
	Values []string `json:"values,omitempty"`
	// For union types
	Variants []Schema `json:"variants,omitempty"`
}

// Field is one property of an object schema.
type Field struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Required    bool     `json:"required"`
	Nullable    bool     `json:"nullable,omitempty"`
	Default     any      `json:"default,omitempty"`
	Description string   `json:"description,omitempty"`
	Validations []string `json:"validations,omitempty"`
	Values      []string `json:"values,omitempty"` // for enum fields
	Fields      []Field  `json:"fields,omitempty"` // for nested objects
	Items       *Schema  `json:"items,omitempty"`  // for array fields
}

// resolveExtractor finds the extractor script. It checks, in order:
//  1. ExtractorOverride (set in tests)
//  2. Relative to the running binary's directory
//  3. Relative to the current working directory
//  4. Falls back to the embedded copy written to a temp file
var ExtractorOverride = ""

// cachedExtractor stores the temp file path so we only write it once per process.
var cachedExtractor string

func resolveExtractor() (string, error) {
	if ExtractorOverride != "" {
		return ExtractorOverride, nil
	}

	// Try relative to binary
	if exe, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(exe), "extractor", "index.js")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}

	// Try relative to CWD
	if cwd, err := os.Getwd(); err == nil {
		candidate := filepath.Join(cwd, "extractor", "index.js")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}

	// Fall back to embedded script
	if cachedExtractor != "" {
		return cachedExtractor, nil
	}

	tmp, err := os.CreateTemp("", "rute-extractor-*.js")
	if err != nil {
		return "", fmt.Errorf("failed to create temp extractor: %w", err)
	}
	if _, err := tmp.Write(extractor.Script); err != nil {
		tmp.Close()
		return "", fmt.Errorf("failed to write temp extractor: %w", err)
	}
	tmp.Close()
	cachedExtractor = tmp.Name()
	return cachedExtractor, nil
}

// Parse runs the Node.js extractor for the given schema reference and returns
// the parsed Schema. baseDir is the directory containing rute.yaml.
func Parse(baseDir, file, export string) (*Schema, error) {
	absFile := filepath.Join(baseDir, file)
	extractorAbs, err := resolveExtractor()
	if err != nil {
		return nil, err
	}

	cmd := exec.Command("node", extractorAbs, absFile, export)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("extractor failed for %s#%s: %s", file, export, string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("extractor error for %s#%s: %w", file, export, err)
	}

	var schema Schema
	if err := json.Unmarshal(out, &schema); err != nil {
		return nil, fmt.Errorf("extractor returned invalid JSON for %s#%s: %w", file, export, err)
	}

	return &schema, nil
}
