// Package renderer renders parsed schemas and endpoints as terminal output.
package renderer

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/sandbye/rute/internal/parser"
)

var (
	styleGET    = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Bold(true)
	stylePOST   = lipgloss.NewStyle().Foreground(lipgloss.Color("4")).Bold(true)
	stylePUT    = lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Bold(true)
	stylePATCH  = lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Bold(true)
	styleDELETE = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true)
	styleDim    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	styleBold   = lipgloss.NewStyle().Bold(true)
	styleTag    = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
)

// MethodStyle returns a coloured method badge string.
func MethodStyle(method string) string {
	switch strings.ToUpper(method) {
	case "GET":
		return styleGET.Render(method)
	case "POST":
		return stylePOST.Render(method)
	case "PUT":
		return stylePUT.Render(method)
	case "PATCH":
		return stylePATCH.Render(method)
	case "DELETE":
		return styleDELETE.Render(method)
	default:
		return method
	}
}

// RenderSchema renders a schema as a terminal tree, returning the string.
func RenderSchema(schema *parser.Schema, indent string, last bool) string {
	if schema == nil {
		return ""
	}
	if schema.Type == "object" {
		var sb strings.Builder
		for i, f := range schema.Fields {
			isLast := i == len(schema.Fields)-1
			sb.WriteString(renderField(f, indent, isLast))
		}
		return sb.String()
	}
	return fmt.Sprintf("%s  %s\n", indent, RenderSchemaTypeSummary(schema))
}

func renderField(f parser.Field, indent string, last bool) string {
	connector := "├── "
	childIndent := indent + "│   "
	if last {
		connector = "└── "
		childIndent = indent + "    "
	}

	typePart := RenderFieldTypeSummary(f)
	meta := make([]string, 0, len(f.Validations)+3)
	if !f.Required {
		meta = append(meta, styleDim.Render("optional"))
	}
	if f.Nullable {
		meta = append(meta, styleDim.Render("nullable"))
	}
	if HasDefaultValue(f.Default) {
		meta = append(meta, styleTag.Render("default:"+FormatDefaultValue(f.Default)))
	}
	if len(f.Validations) > 0 {
		meta = append(meta, styleTag.Render(strings.Join(f.Validations, " ")))
	}

	metaPart := ""
	if len(meta) > 0 {
		metaPart = "  " + strings.Join(meta, "  ")
	}

	desc := ""
	if f.Description != "" {
		desc = "  " + styleDim.Render(f.Description)
	}

	line := fmt.Sprintf("%s%s%s  %s%s%s\n",
		indent, connector,
		styleBold.Render(f.Name),
		typePart,
		metaPart,
		desc,
	)

	if len(f.Fields) > 0 {
		var sb strings.Builder
		sb.WriteString(line)
		for i, child := range f.Fields {
			isLast := i == len(f.Fields)-1
			sb.WriteString(renderField(child, childIndent, isLast))
		}
		return sb.String()
	}

	return line
}

// RenderFieldTypeSummary returns a compact, human-readable type summary.
func RenderFieldTypeSummary(f parser.Field) string {
	return renderTypeSummary(f.Type, f.Values, f.Items, f.Variants)
}

// RenderSchemaTypeSummary returns a compact, human-readable type summary.
func RenderSchemaTypeSummary(schema *parser.Schema) string {
	if schema == nil {
		return ""
	}
	return renderTypeSummary(schema.Type, schema.Values, schema.Items, schema.Variants)
}

// HasDefaultValue reports whether a field has a default value.
func HasDefaultValue(v any) bool {
	return v != nil
}

// FormatDefaultValue renders a default value while preserving falsey values.
func FormatDefaultValue(v any) string {
	switch value := v.(type) {
	case string:
		return fmt.Sprintf("%q", value)
	case nil:
		return ""
	default:
		data, err := json.Marshal(value)
		if err == nil {
			return string(data)
		}
		return fmt.Sprint(value)
	}
}

func renderTypeSummary(typ string, values []string, items *parser.Schema, variants []parser.Schema) string {
	switch typ {
	case "enum":
		quoted := make([]string, len(values))
		for i, v := range values {
			quoted[i] = fmt.Sprintf("%q", v)
		}
		return "enum  " + strings.Join(quoted, " | ")
	case "array":
		if items != nil {
			return fmt.Sprintf("array<%s>", inlineSchemaSummary(items))
		}
		return "array"
	case "record":
		if items != nil {
			return fmt.Sprintf("record<%s>", inlineSchemaSummary(items))
		}
		return "record"
	case "union":
		if len(variants) > 0 {
			parts := make([]string, len(variants))
			for i, variant := range variants {
				parts[i] = inlineSchemaSummary(&variant)
			}
			return "union<" + strings.Join(parts, " | ") + ">"
		}
		return "union"
	case "intersection":
		if len(variants) > 0 {
			parts := make([]string, len(variants))
			for i, variant := range variants {
				parts[i] = inlineSchemaSummary(&variant)
			}
			return "intersection<" + strings.Join(parts, " & ") + ">"
		}
		return "intersection"
	case "literal":
		if len(values) > 0 {
			return fmt.Sprintf("literal  %q", values[0])
		}
		return "literal"
	default:
		return typ
	}
}

func inlineSchemaSummary(schema *parser.Schema) string {
	if schema == nil {
		return "unknown"
	}
	switch schema.Type {
	case "object":
		return "object"
	case "enum", "array", "record", "union", "intersection", "literal":
		return renderTypeSummary(schema.Type, schema.Values, schema.Items, schema.Variants)
	default:
		return schema.Type
	}
}
