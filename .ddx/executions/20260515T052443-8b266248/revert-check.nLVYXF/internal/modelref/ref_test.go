package modelref

import (
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantRef ModelRef
		wantErr bool
	}{
		{
			name:    "simple",
			input:   "openrouter/claude-3-opus",
			wantRef: ModelRef{Provider: "openrouter", ID: "claude-3-opus"},
		},
		{
			name:    "id with slashes (sub-path)",
			input:   "openrouter/qwen/qwen3.6-27b",
			wantRef: ModelRef{Provider: "openrouter", ID: "qwen/qwen3.6-27b"},
		},
		{
			name:    "id with multiple slashes",
			input:   "openrouter/anthropic/claude-opus-4-5",
			wantRef: ModelRef{Provider: "openrouter", ID: "anthropic/claude-opus-4-5"},
		},
		{
			name:    "provider with hyphen",
			input:   "vidar-ds4/deepseek-v4-flash",
			wantRef: ModelRef{Provider: "vidar-ds4", ID: "deepseek-v4-flash"},
		},
		{
			name:    "provider with dot",
			input:   "sindri.club/llama3-70b",
			wantRef: ModelRef{Provider: "sindri.club", ID: "llama3-70b"},
		},
		{
			name:    "provider with digits",
			input:   "ollama3/llama3",
			wantRef: ModelRef{Provider: "ollama3", ID: "llama3"},
		},
		{
			name:    "no slash → error",
			input:   "openrouter",
			wantErr: true,
		},
		{
			name:    "empty provider (leading slash)",
			input:   "/claude-3",
			wantErr: true,
		},
		{
			name:    "empty id (trailing slash)",
			input:   "openrouter/",
			wantErr: true,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "provider with uppercase letter",
			input:   "OpenRouter/model",
			wantErr: true,
		},
		{
			name:    "provider with space",
			input:   "open router/model",
			wantErr: true,
		},
		{
			name:    "provider with underscore",
			input:   "open_router/model",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("Parse(%q): expected error, got %v", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Errorf("Parse(%q): unexpected error: %v", tt.input, err)
				return
			}
			if got != tt.wantRef {
				t.Errorf("Parse(%q) = %+v, want %+v", tt.input, got, tt.wantRef)
			}
		})
	}
}

func TestString(t *testing.T) {
	r := ModelRef{Provider: "openrouter", ID: "qwen/qwen3.6-27b"}
	got := r.String()
	want := "openrouter/qwen/qwen3.6-27b"
	if got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestStringRoundtrip(t *testing.T) {
	inputs := []string{
		"openrouter/claude-3-opus",
		"openrouter/qwen/qwen3.6-27b",
		"vidar-ds4/deepseek-v4-flash",
		"ollama/llama3:8b",
	}
	for _, s := range inputs {
		r, err := Parse(s)
		if err != nil {
			continue // skip invalid
		}
		if got := r.String(); got != s {
			t.Errorf("Parse(%q).String() = %q, want %q", s, got, s)
		}
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		ref     ModelRef
		wantErr bool
	}{
		{"valid simple", ModelRef{Provider: "openrouter", ID: "claude"}, false},
		{"valid with dot", ModelRef{Provider: "sindri.club", ID: "model"}, false},
		{"valid with hyphen", ModelRef{Provider: "vidar-ds4", ID: "model"}, false},
		{"empty provider", ModelRef{Provider: "", ID: "model"}, true},
		{"empty id", ModelRef{Provider: "openrouter", ID: ""}, true},
		{"provider with slash", ModelRef{Provider: "open/router", ID: "model"}, true},
		{"provider uppercase", ModelRef{Provider: "OpenRouter", ID: "model"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.ref.Validate()
			if tt.wantErr && err == nil {
				t.Errorf("Validate(%+v): expected error", tt.ref)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("Validate(%+v): unexpected error: %v", tt.ref, err)
			}
		})
	}
}
