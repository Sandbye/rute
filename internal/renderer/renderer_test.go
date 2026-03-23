package renderer

import (
	"strings"
	"testing"

	"github.com/sandbye/rute/internal/parser"
)

func TestRenderSchemaShowsNullableDefaultAndComplexTypes(t *testing.T) {
	schema := &parser.Schema{
		Type: "object",
		Fields: []parser.Field{
			{
				Name:     "role",
				Type:     "enum",
				Required: false,
				Default:  "user",
				Values:   []string{"admin", "user"},
			},
			{
				Name:     "meta",
				Type:     "record",
				Required: true,
				Items:    &parser.Schema{Type: "string"},
			},
			{
				Name:     "result",
				Type:     "union",
				Required: true,
				Nullable: true,
				Variants: []parser.Schema{{Type: "string"}, {Type: "number"}},
			},
		},
	}

	out := RenderSchema(schema, "", true)

	for _, want := range []string{
		`default:"user"`,
		`record<string>`,
		`union<string | number>`,
		`nullable`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected output to contain %q, got:\n%s", want, out)
		}
	}
}

func TestFormatDefaultValueKeepsFalseyValues(t *testing.T) {
	tests := map[string]any{
		`false`: false,
		`0`:     0,
		`""`:    "",
	}

	for want, input := range tests {
		if got := FormatDefaultValue(input); got != want {
			t.Fatalf("expected %q, got %q", want, got)
		}
	}
}
