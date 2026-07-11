package compiler

import "testing"

func TestDirectiveArgs(t *testing.T) {
	tests := []struct {
		name      string
		directive directive
		want      []string
		wantError bool
	}{
		{name: "command arguments", directive: directive{kind: "CMD", args: "commit --help"}, want: []string{"commit", "--help"}},
		{name: "exact tool prefix", directive: directive{kind: "CMD", args: "git commit --help"}, want: []string{"commit", "--help"}},
		{name: "subcommand help", directive: directive{kind: "SUBCMD", args: "remote"}, want: []string{"remote", "--help"}},
		{name: "reject shell syntax", directive: directive{kind: "CMD", args: "--help; whoami"}, wantError: true},
		{name: "reject empty command", directive: directive{kind: "CMD", args: "   "}, wantError: true},
		{name: "reject prefix only", directive: directive{kind: "CMD", args: "git"}, wantError: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := directiveArgs(test.directive, "git")
			if test.wantError {
				if err == nil {
					t.Fatal("directiveArgs() returned nil error")
				}
				return
			}
			if err != nil {
				t.Fatalf("directiveArgs() error = %v", err)
			}
			if len(got) != len(test.want) {
				t.Fatalf("directiveArgs() = %#v, want %#v", got, test.want)
			}
			for index := range got {
				if got[index] != test.want[index] {
					t.Fatalf("directiveArgs() = %#v, want %#v", got, test.want)
				}
			}
		})
	}
}

func TestFormatToolResult(t *testing.T) {
	got := formatToolResult("git --help", "usage: git <command>", "warning", nil)
	want := "<TOOL_RESULT>\nCOMMAND: git --help\nEXIT: 0\n--- BEGIN STDOUT ---\nusage: git <command>\n--- END STDOUT ---\n--- BEGIN STDERR ---\nwarning\n--- END STDERR ---\n</TOOL_RESULT>"
	if got != want {
		t.Fatalf("formatToolResult() = %q, want %q", got, want)
	}
}
