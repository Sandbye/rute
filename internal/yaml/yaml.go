// Package yaml reads and parses rute.yaml into typed Go structs.
package yaml

import (
	"fmt"
	"os"
	"strings"

	goyaml "gopkg.in/yaml.v3"
)

// Config is the top-level rute.yaml structure.
type Config struct {
	Title       string     `yaml:"title"`
	Version     string     `yaml:"version"`
	BaseURL     string     `yaml:"baseUrl"`
	Description string     `yaml:"description"`
	Endpoints   []Endpoint `yaml:"endpoints"`
}

// Endpoint describes a single API route.
type Endpoint struct {
	Path        string                  `yaml:"path"`
	Method      string                  `yaml:"method"`
	Description string                  `yaml:"description"`
	Tags        []string                `yaml:"tags"`
	Params      *SchemaHolder           `yaml:"params"`
	Query       *SchemaHolder           `yaml:"query"`
	Body        *SchemaHolder           `yaml:"body"`
	Response    map[string]SchemaHolder `yaml:"response"`
	Examples    []Example               `yaml:"examples"`
}

// SchemaHolder wraps a single schema reference.
type SchemaHolder struct {
	Schema string `yaml:"schema"`
}

// Example is a code snippet shown in exported docs.
type Example struct {
	Lang  string `yaml:"lang"`
	Label string `yaml:"label"`
	Code  string `yaml:"code"`
}

// SchemaRef holds the parsed components of a schema reference string.
// Format: "./path/to/file.ts#ExportedSchemaName"
type SchemaRef struct {
	File   string
	Export string
	Raw    string
}

// ParseSchemaRef parses a schema reference string into its components.
func ParseSchemaRef(ref string) (SchemaRef, error) {
	parts := strings.SplitN(ref, "#", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return SchemaRef{}, fmt.Errorf("invalid schema ref %q: expected format ./path/to/file.ts#ExportName", ref)
	}
	if !strings.HasSuffix(parts[0], ".ts") {
		return SchemaRef{}, fmt.Errorf("invalid schema ref %q: file path must end in .ts", ref)
	}
	return SchemaRef{File: parts[0], Export: parts[1], Raw: ref}, nil
}

// Load reads and parses a rute.yaml file.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("could not read %s: %w", path, err)
	}

	var cfg Config
	if err := goyaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("could not parse %s: %w", path, err)
	}

	if err := validate(&cfg, path); err != nil {
		return nil, err
	}

	// Normalise method to uppercase.
	for i := range cfg.Endpoints {
		cfg.Endpoints[i].Method = strings.ToUpper(cfg.Endpoints[i].Method)
	}

	return &cfg, nil
}

func validate(cfg *Config, path string) error {
	if cfg.Title == "" {
		return fmt.Errorf("%s: missing required field: title", path)
	}
	if cfg.Version == "" {
		return fmt.Errorf("%s: missing required field: version", path)
	}
	if len(cfg.Endpoints) == 0 {
		return fmt.Errorf("%s: missing required field: endpoints (must have at least one)", path)
	}
	for i, ep := range cfg.Endpoints {
		if ep.Path == "" {
			return fmt.Errorf("%s: endpoint[%d]: missing required field: path", path, i)
		}
		if ep.Method == "" {
			return fmt.Errorf("%s: endpoint[%d]: missing required field: method", path, i)
		}
		if !strings.HasPrefix(ep.Path, "/") {
			return fmt.Errorf("%s: endpoint[%d]: path must start with /", path, i)
		}
	}
	return nil
}
