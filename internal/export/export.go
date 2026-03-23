// Package export generates a self-contained static HTML documentation site.
package export

import (
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sandbye/rute/internal/parser"
	"github.com/sandbye/rute/internal/renderer"
	ruteYaml "github.com/sandbye/rute/internal/yaml"
)

// EndpointData holds a fully resolved endpoint for the HTML template.
type EndpointData struct {
	Path        string
	Method      string
	Description string
	Tags        []string
	Params      *parser.Schema
	Query       *parser.Schema
	Body        *parser.Schema
	Responses   []ResponseData
	Examples    []ruteYaml.Example
	ID          string // anchor ID
}

// ResponseData is a status code + schema pair.
type ResponseData struct {
	Code   string
	Schema *parser.Schema
}

// Group is a collection of endpoints under a common label.
type Group struct {
	Name      string
	Endpoints []EndpointData
}

// SiteData is the top-level data passed to the HTML template.
type SiteData struct {
	Title       string
	Version     string
	BaseURL     string
	Description string
	Groups      []Group
}

// Generate resolves all schemas and writes a single index.html.
func Generate(cfg *ruteYaml.Config, baseDir, outDir string) error {
	endpoints := make([]EndpointData, 0, len(cfg.Endpoints))

	for i, ep := range cfg.Endpoints {
		ed := EndpointData{
			Path:        ep.Path,
			Method:      ep.Method,
			Description: ep.Description,
			Tags:        ep.Tags,
			ID:          fmt.Sprintf("ep-%d", i),
		}

		if ep.Params != nil && ep.Params.Schema != "" {
			ed.Params = resolveSchema(baseDir, ep.Params.Schema)
		}
		if ep.Query != nil && ep.Query.Schema != "" {
			ed.Query = resolveSchema(baseDir, ep.Query.Schema)
		}
		if ep.Body != nil && ep.Body.Schema != "" {
			ed.Body = resolveSchema(baseDir, ep.Body.Schema)
		}
		if len(ep.Response) > 0 {
			codes := make([]string, 0, len(ep.Response))
			for code := range ep.Response {
				codes = append(codes, code)
			}
			sort.Strings(codes)
			for _, code := range codes {
				holder := ep.Response[code]
				var s *parser.Schema
				if holder.Schema != "" {
					s = resolveSchema(baseDir, holder.Schema)
				}
				ed.Responses = append(ed.Responses, ResponseData{Code: code, Schema: s})
			}
		}

		endpoints = append(endpoints, ed)
	}

	groups := groupEndpoints(endpoints)

	data := SiteData{
		Title:       cfg.Title,
		Version:     cfg.Version,
		BaseURL:     cfg.BaseURL,
		Description: cfg.Description,
		Groups:      groups,
	}

	if err := os.MkdirAll(outDir, 0755); err != nil {
		return fmt.Errorf("failed to create output dir: %w", err)
	}

	outPath := filepath.Join(outDir, "index.html")
	f, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("failed to create %s: %w", outPath, err)
	}
	defer f.Close()

	tmpl, err := template.New("site").Funcs(templateFuncs).Parse(htmlTemplate)
	if err != nil {
		return fmt.Errorf("template parse error: %w", err)
	}

	if err := tmpl.Execute(f, data); err != nil {
		return fmt.Errorf("template execute error: %w", err)
	}

	return nil
}

// groupEndpoints groups by first tag, or by first path segment if no tags.
func groupEndpoints(endpoints []EndpointData) []Group {
	grouped := make(map[string][]EndpointData)
	order := []string{}

	for _, ep := range endpoints {
		var group string
		if len(ep.Tags) > 0 {
			group = ep.Tags[0]
		} else {
			// Derive from first path segment: /users/:id → users
			parts := strings.Split(strings.TrimPrefix(ep.Path, "/"), "/")
			if len(parts) > 0 {
				group = parts[0]
			} else {
				group = "other"
			}
		}

		if _, exists := grouped[group]; !exists {
			order = append(order, group)
		}
		grouped[group] = append(grouped[group], ep)
	}

	groups := make([]Group, 0, len(order))
	for _, name := range order {
		groups = append(groups, Group{
			Name:      strings.ToUpper(name[:1]) + name[1:],
			Endpoints: grouped[name],
		})
	}
	return groups
}

func resolveSchema(baseDir, ref string) *parser.Schema {
	sr, err := ruteYaml.ParseSchemaRef(ref)
	if err != nil {
		return nil
	}
	absBase, _ := filepath.Abs(baseDir)
	schema, err := parser.Parse(absBase, sr.File, sr.Export)
	if err != nil {
		return nil
	}
	return schema
}

// fieldCtx is passed to the fieldLine template for tree rendering.
type fieldCtx struct {
	Field  parser.Field
	IsLast bool
	Prefix string
}

var templateFuncs = template.FuncMap{
	"makeFieldCtx": func(f parser.Field, idx, total int, prefix string) fieldCtx {
		return fieldCtx{
			Field:  f,
			IsLast: idx == total-1,
			Prefix: prefix,
		}
	},
	"childPrefix": func(parentPrefix string, parentIsLast bool) string {
		if parentIsLast {
			return parentPrefix + "    "
		}
		return parentPrefix + "│   "
	},
	"methodColor": func(method string) string {
		switch method {
		case "GET":
			return "#22c55e"
		case "POST":
			return "#3b82f6"
		case "PUT", "PATCH":
			return "#eab308"
		case "DELETE":
			return "#ef4444"
		default:
			return "#94a3b8"
		}
	},
	"methodBg": func(method string) template.CSS {
		switch method {
		case "GET":
			return "rgba(34,197,94,0.1)"
		case "POST":
			return "rgba(59,130,246,0.1)"
		case "PUT", "PATCH":
			return "rgba(234,179,8,0.1)"
		case "DELETE":
			return "rgba(239,68,68,0.1)"
		default:
			return "rgba(148,163,184,0.1)"
		}
	},
	"fieldType":    renderer.RenderFieldTypeSummary,
	"hasDefault":   renderer.HasDefaultValue,
	"defaultValue": renderer.FormatDefaultValue,
}
