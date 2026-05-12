package main

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func startFizAsync(t *testing.T, fixture fizFixture, readyFile string, args ...string) (*exec.Cmd, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()

	exe := buildFiz(t)
	wrapperArgs := []string{
		"-c",
		`printf started > "$1"; shift; exec "$@"`,
		"sh",
		readyFile,
		exe,
		"--work-dir",
		fixture.workDir,
	}
	wrapperArgs = append(wrapperArgs, args...)

	cmd := exec.Command("/bin/sh", wrapperArgs...)
	cmd.Dir = fixture.workDir
	cmd.Env = fixture.env

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("start fiz: %v", err)
	}
	return cmd, &stdout, &stderr
}

func waitForFile(t *testing.T, path string, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
}

func TestModelsConcurrentRefreshSingleFlight(t *testing.T) {
	fixture := newFizFixture(t)

	root := filepath.Dir(fixture.workDir)
	catalogPath := filepath.Join(root, "models.yaml")

	var requests int32
	firstRequestStarted := make(chan struct{})
	releaseFirstRequest := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		if atomic.AddInt32(&requests, 1) == 1 {
			close(firstRequestStarted)
			<-releaseFirstRequest
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"gpt-5.5"}]}`))
	}))
	defer server.Close()

	config := fmt.Sprintf(`
model_catalog:
  manifest: %s
providers:
  alpha:
    type: openai
    base_url: %s/v1
    billing: fixed
    include_by_default: true
  beta:
    type: openrouter
    billing: fixed
    include_by_default: true
`, catalogPath, server.URL)
	if err := os.WriteFile(filepath.Join(fixture.workDir, ".fizeau", "config.yaml"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}

	readyA := filepath.Join(root, "ready-a")
	readyB := filepath.Join(root, "ready-b")
	cmdA, stdoutA, stderrA := startFizAsync(t, fixture, readyA, "models", "--refresh")
	cmdB, stdoutB, stderrB := startFizAsync(t, fixture, readyB, "models", "--refresh")
	t.Cleanup(func() {
		for _, cmd := range []*exec.Cmd{cmdA, cmdB} {
			if cmd != nil && cmd.Process != nil && cmd.ProcessState == nil {
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
			}
		}
	})

	waitForFile(t, readyA, 10*time.Second)
	waitForFile(t, readyB, 10*time.Second)

	select {
	case <-firstRequestStarted:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for the first discovery request")
	}

	time.Sleep(200 * time.Millisecond)
	if got := atomic.LoadInt32(&requests); got != 1 {
		t.Fatalf("duplicate discovery IO before release: got %d requests", got)
	}

	close(releaseFirstRequest)

	if err := cmdA.Wait(); err != nil {
		t.Fatalf("first fiz invocation failed: %v\nstderr=%s\nstdout=%s", err, stderrA.String(), stdoutA.String())
	}
	if err := cmdB.Wait(); err != nil {
		t.Fatalf("second fiz invocation failed: %v\nstderr=%s\nstdout=%s", err, stderrB.String(), stdoutB.String())
	}

	if got := atomic.LoadInt32(&requests); got != 1 {
		t.Fatalf("concurrent refresh duplicated discovery IO: got %d requests\nstdoutA=%s\nstdoutB=%s", got, stdoutA.String(), stdoutB.String())
	}

	for _, want := range []string{"alpha", "beta", "gpt-5.5", "claude-opus-4.5"} {
		if !strings.Contains(stdoutA.String(), want) && !strings.Contains(stdoutB.String(), want) {
			t.Fatalf("expected %q in multi-provider models output:\nstdoutA=%s\nstdoutB=%s", want, stdoutA.String(), stdoutB.String())
		}
	}
}
