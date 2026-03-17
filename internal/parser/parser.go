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

// ExtractorOverride is set in tests to use a specific extractor path.
var ExtractorOverride = ""

// Cached temp file paths (written once per process).
var cachedRuntime string
var cachedStatic string

// runtimeDisabled is set to true after the first runtime FALLBACK,
// so we don't keep retrying for every schema ref in a single run.
var runtimeDisabled bool

func embeddedScript(cached *string, content []byte, prefix string) (string, error) {
	if *cached != "" {
		return *cached, nil
	}
	tmp, err := os.CreateTemp("", prefix)
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return "", fmt.Errorf("failed to write temp file: %w", err)
	}
	tmp.Close()
	*cached = tmp.Name()
	return *cached, nil
}

// Parse runs the Node.js extractor for the given schema reference and returns
// the parsed Schema. baseDir is the directory containing rute.yaml.
//
// It tries the runtime extractor first (esbuild + z.toJSONSchema), which
// handles all Zod patterns perfectly. If the runtime extractor signals a
// fallback (exit code 2 — missing esbuild/zod/node_modules), it falls
// back to the static regex-based extractor.
func Parse(baseDir, file, export string) (*Schema, error) {
	absFile := filepath.Join(baseDir, file)

	if ExtractorOverride != "" {
		return runExtractor(ExtractorOverride, absFile, file, export)
	}

	// Try runtime extractor first (unless previously disabled).
	if !runtimeDisabled {
		runtimePath, err := embeddedScript(&cachedRuntime, extractor.RuntimeScript, "rute-runtime-*.js")
		if err == nil {
			schema, err := runExtractor(runtimePath, absFile, file, export)
			if err == nil {
				return schema, nil
			}
			// Check if the runtime signalled fallback (exit code 2).
			if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 2 {
				runtimeDisabled = true
				// Fall through to static extractor.
			} else {
				// Real error from runtime — don't fall back, report it.
				return nil, err
			}
		}
	}

	// Fall back to static extractor.
	staticPath, err := embeddedScript(&cachedStatic, extractor.Script, "rute-static-*.js")
	if err != nil {
		return nil, err
	}
	return runExtractor(staticPath, absFile, file, export)
}

func runExtractor(extractorPath, absFile, file, export string) (*Schema, error) {
	cmd := exec.Command("node", extractorPath, absFile, export)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			// Return the raw error so caller can inspect exit code.
			if exitErr.ExitCode() == 2 {
				return nil, exitErr
			}
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
