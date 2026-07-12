package cli

import (
	"testing"

	openapi "codeflower-runner/internal/core/client"
)

func TestInferVariantByArguments(t *testing.T) {
	prefix := "--input"
	command := openapi.CommandSpecSchema{
		Variants: []openapi.CommandVariantSpecSchema{
			{
				Name: "required-input",
				Parameters: []openapi.ParamSpecSchema{
					{Prefix: &prefix, Name: "input", Type: openapi.ParameterTypePath, IsRequired: true},
				},
			},
			{
				Name:       "no-input",
				Parameters: []openapi.ParamSpecSchema{},
			},
		},
	}

	variant, err := inferVariant(&command, []string{"--input", "reads.fastq"})
	if err != nil {
		t.Fatalf("inferVariant returned error: %v", err)
	}
	if variant.Name != "required-input" {
		t.Fatalf("selected variant %q, want required-input", variant.Name)
	}
}

func TestToolBinaryPath(t *testing.T) {
	if got := toolBinaryPath("@lab/My Tool:v1.2"); got != "/app/my-tool" {
		t.Fatalf("toolBinaryPath returned %q, want /app/my-tool", got)
	}
}

func TestInferVariantByPositionalArguments(t *testing.T) {
	command := openapi.CommandSpecSchema{
		Variants: []openapi.CommandVariantSpecSchema{
			{
				Name: "with-reference",
				Parameters: []openapi.ParamSpecSchema{
					{Name: "reference", Type: openapi.ParameterTypeString, IsRequired: true, Position: intPointer(0)},
				},
			},
			{
				Name:       "without-reference",
				Parameters: []openapi.ParamSpecSchema{},
			},
		},
	}

	variant, err := inferVariant(&command, []string{"reference.fa"})
	if err != nil {
		t.Fatalf("inferVariant returned error: %v", err)
	}
	if variant.Name != "with-reference" {
		t.Fatalf("selected variant %q, want with-reference", variant.Name)
	}
}

func intPointer(value int) *int {
	return &value
}
