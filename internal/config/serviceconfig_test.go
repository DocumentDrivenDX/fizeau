package config

import "testing"

func TestServiceConfigSessionLogDir(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		baseDir string
		want    string
	}{
		{
			name:    "default falls back to <workDir>/.fizeau/sessions",
			cfg:     &Config{},
			baseDir: "/work",
			want:    "/work/.fizeau/sessions",
		},
		{
			name:    "absolute configured path is honored as-is",
			cfg:     &Config{SessionLogDir: "/logs/agent/sessions"},
			baseDir: "/work",
			want:    "/logs/agent/sessions",
		},
		{
			name:    "relative configured path resolves against workDir",
			cfg:     &Config{SessionLogDir: "custom/sessions"},
			baseDir: "/work",
			want:    "/work/custom/sessions",
		},
		{
			name:    "absolute configured path with empty workDir still works",
			cfg:     &Config{SessionLogDir: "/logs/agent/sessions"},
			baseDir: "",
			want:    "/logs/agent/sessions",
		},
		{
			name:    "relative configured path with empty workDir returns the relative value",
			cfg:     &Config{SessionLogDir: "rel/sessions"},
			baseDir: "",
			want:    "rel/sessions",
		},
		{
			name:    "no config and no workDir returns empty",
			cfg:     &Config{},
			baseDir: "",
			want:    "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := &configServiceConfig{cfg: tc.cfg, baseDir: tc.baseDir}
			if got := c.SessionLogDir(); got != tc.want {
				t.Fatalf("SessionLogDir() = %q, want %q", got, tc.want)
			}
		})
	}
}
