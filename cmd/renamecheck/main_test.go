package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestFailFlagExitsNonZeroForForbiddenSurface(t *testing.T) {
	cases := map[string]struct {
		path    string
		content string
	}{
		"old module path": {
			path:    "README.md",
			content: "module github.com/DocumentDrivenDX/" + "agent" + "\n",
		},
		"root package": {
			path:    "agent.go",
			content: "package " + "agent" + "\n",
		},
		"binary name": {
			path:    "README.md",
			content: "Run ddx-" + "agent" + ".\n",
		},
		"product name all caps": {
			path:    "README.md",
			content: "Launch DDX" + " Agent" + ".\n",
		},
		"product name mixed caps": {
			path:    "README.md",
			content: "Launch DDx" + " Agent" + ".\n",
		},
		"state directory": {
			path:    "config.yaml",
			content: "path: ." + "agent/session.jsonl\n",
		},
		"agent env": {
			path:    "config.env",
			content: "AGENT" + "_DEBUG=1\n",
		},
		"ddx agent env": {
			path:    "config.env",
			content: "DDX_" + "AGENT" + "_DEBUG=1\n",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, tc.path), []byte(tc.content), 0o644); err != nil {
				t.Fatalf("WriteFile() error = %v", err)
			}

			cmd := exec.Command("go", "run", ".", "--repo", root, "--fail")
			cmd.Dir = "."
			err := cmd.Run()
			if err == nil {
				t.Fatal("renamecheck --fail exited successfully, want failure")
			}
			if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 1 {
				t.Fatalf("renamecheck --fail error = %v, want exit code 1", err)
			}
		})
	}
}

func TestFailFlagExitsZeroForCleanTree(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("Run fiz.\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cmd := exec.Command("go", "run", ".", "--repo", root, "--fail")
	cmd.Dir = "."
	if err := cmd.Run(); err != nil {
		t.Fatalf("renamecheck --fail error = %v, want success", err)
	}
}
