// Package renderer renders parsed schemas and endpoints as terminal output.
package renderer

import (
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
	// Top-level call renders the object fields directly.
	if schema.Type == "object" {
		var sb strings.Builder
		for i, f := range schema.Fields {
			isLast := i == len(schema.Fields)-1
			sb.WriteString(renderField(f, indent, isLast))
		}
		return sb.String()
	}
	return fmt.Sprintf("%s  %s\n", indent, renderTypeSummary(schema.Type, schema.Values, nil))
}

func renderField(f parser.Field, indent string, last bool) string {
	connector := "├── "
	childIndent := indent + "│   "
	if last {
		connector = "└── "
		childIndent = indent + "    "
	}

	req := styleDim.Render("optional")
	if f.Required {
		req = ""
	}

	typePart := renderTypeSummary(f.Type, f.Values, f.Items)
	validations := ""
	if len(f.Validations) > 0 {
		validations = "  " + styleTag.Render(strings.Join(f.Validations, " "))
	}
	desc := ""
	if f.Description != "" {
		desc = "  " + styleDim.Render(f.Description)
	}

	reqPart := ""
	if req != "" {
		reqPart = "  " + req
	}

	line := fmt.Sprintf("%s%s%s  %s%s%s%s\n",
		indent, connector,
		styleBold.Render(f.Name),
		typePart,
		reqPart,
		validations,
		desc,
	)

	// Recurse into nested objects.
	if f.Type == "object" && len(f.Fields) > 0 {
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

func renderTypeSummary(typ string, values []string, items *parser.Schema) string {
	switch typ {
	case "enum":
		quoted := make([]string, len(values))
		for i, v := range values {
			quoted[i] = fmt.Sprintf("%q", v)
		}
		return "enum  " + strings.Join(quoted, " | ")
	case "array":
		if items != nil {
			return fmt.Sprintf("array<%s>", items.Type)
		}
		return "array"
	default:
		return typ
	}
}
