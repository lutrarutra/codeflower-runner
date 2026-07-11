package compiler

// CompileResult is the top-level output of the reverse-engineering process.
// It maps to the YAML that the Go tool prints to stdout.
type CompileResult struct {
	Subcommands []SubcommandSpec `yaml:"subcommands"`
}

// SubcommandSpec describes a single CLI subcommand and its parameters.
type SubcommandSpec struct {
	Name        string      `yaml:"name"`
	Description string      `yaml:"description"`
	BaseCommand []string    `yaml:"baseCommand"`
	Inputs      []ParamSpec `yaml:"inputs"`
	Outputs     []ParamSpec `yaml:"outputs"`
}

// ParamSpec describes a single input or output parameter.
type ParamSpec struct {
	Name         string   `yaml:"name"`
	Type         string   `yaml:"type"`                   // File, string, int, float, boolean, Directory
	Description  string   `yaml:"description"`
	Prefix       string   `yaml:"prefix,omitempty"`
	Position     *int     `yaml:"position,omitempty"`
	IsRequired   bool     `yaml:"isRequired"`
	IsArray      bool     `yaml:"isArray"`
	DefaultValue string   `yaml:"defaultValue,omitempty"`
	AllowedValues []string `yaml:"allowedValues,omitempty"`
	PathRole     string   `yaml:"pathRole,omitempty"` // "input" or "output"
}
