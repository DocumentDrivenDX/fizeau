package main

// runtime_props_import_test.go demonstrates the import path from a server's
// /props endpoint all the way into a benchmark-evidence record that passes
// schema validation:
//
//   captured /props JSON  →  httptest server
//                          →  runtimeprops.Extract(LaneInfo)
//                          →  evidence.RuntimeProps struct
//                          →  embedded in a benchmark-evidence record
//                          →  jsonschema.Validate (PASS)
//
// The fixtures under internal/benchmark/runtimeprops/testdata/ are byte-for-
// byte captures from the live servers (vidar:1236 ds4, sindri:1236 lucebox).
// If a server's /props shape changes, re-capture with `curl <host>/props |
// python3 -m json.tool > <fixture>` and update assertions as needed.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/easel/fizeau/internal/benchmark/evidence"
	"github.com/easel/fizeau/internal/benchmark/runtimeprops"
)

func TestRuntimePropsImportToCellEvidence(t *testing.T) {
	repoRoot := benchRepoRoot(t)
	fixturesDir := filepath.Join(repoRoot, "internal", "benchmark", "runtimeprops", "testdata")
	schema := compileBenchmarkEvidenceSchema(t)

	cases := []struct {
		name        string
		fixture     string
		laneRuntime string
		laneModel   string
		provider    string
	}{
		{
			name:        "ds4 captured props",
			fixture:     "ds4-props.json",
			laneRuntime: "ds4",
			laneModel:   "deepseek-v4-flash",
			provider:    "ds4",
		},
		{
			name:        "lucebox captured props",
			fixture:     "lucebox-props.json",
			laneRuntime: "lucebox",
			laneModel:   "luce-dflash",
			provider:    "lucebox",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			// Step 1: load the captured /props body.
			body, err := os.ReadFile(filepath.Join(fixturesDir, tc.fixture))
			if err != nil {
				t.Fatalf("read fixture %s: %v", tc.fixture, err)
			}

			// Step 2: serve it from an httptest server at /props.
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/props" {
					http.NotFound(w, r)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(body)
			}))
			defer srv.Close()

			// Step 3: run the extractor.
			lane := runtimeprops.LaneInfo{
				Runtime: tc.laneRuntime,
				BaseURL: srv.URL + "/v1",
				Model:   tc.laneModel,
			}
			props, err := runtimeprops.Extract(context.Background(), lane)
			if err != nil {
				t.Fatalf("Extract: %v", err)
			}
			if props.Extractor != tc.laneRuntime {
				t.Errorf("extractor = %q, want %q", props.Extractor, tc.laneRuntime)
			}

			// Step 4: prove the import is lossless — the raw /props response is
			// preserved in platform_raw so any downstream analyzer or future
			// extractor enhancement can recover fields the current extractor
			// doesn't yet surface as typed fields.
			if props.PlatformRaw == nil {
				t.Fatal("platform_raw is nil; the import path must preserve the full /props response")
			}
			rawProps, ok := props.PlatformRaw["props"]
			if !ok {
				t.Fatal("platform_raw missing 'props' key; raw /props response was not preserved")
			}
			// Sanity check: a few unambiguous fields from each fixture should be
			// reachable through platform_raw.
			rawMap, ok := rawProps.(map[string]any)
			if !ok {
				t.Fatalf("platform_raw[\"props\"] is %T, want map[string]any", rawProps)
			}
			if _, ok := rawMap["model"]; !ok {
				t.Errorf("platform_raw[\"props\"][\"model\"] missing — fixture preserved lossily")
			}

			// Step 5: embed the RuntimeProps in a complete benchmark-evidence
			// record and schema-validate. This is the "imported to cells" step:
			// the same struct that the bench runner stamps onto cell evidence
			// records must round-trip through JSON and pass the schema.
			record := buildRuntimePropsEvidenceRecord(tc.provider, tc.laneModel, props)
			encoded, err := json.Marshal(record)
			if err != nil {
				t.Fatalf("marshal evidence record: %v", err)
			}

			var doc map[string]any
			dec := json.NewDecoder(bytes.NewReader(encoded))
			dec.UseNumber()
			if err := dec.Decode(&doc); err != nil {
				t.Fatalf("decode evidence record: %v", err)
			}
			if err := schema.Validate(doc); err != nil {
				t.Fatalf("schema validation failed for evidence record built from %s: %v",
					tc.fixture, err)
			}

			// Confirm the embedded extractor name survived the round-trip.
			if extractorName, ok := lookupPath(doc, "runtime_props.extractor"); !ok {
				t.Error("runtime_props.extractor missing from validated record")
			} else if extractorName != tc.laneRuntime {
				t.Errorf("runtime_props.extractor = %v, want %q", extractorName, tc.laneRuntime)
			}
		})
	}
}

// buildRuntimePropsEvidenceRecord assembles a minimal valid benchmark-evidence
// record around an extracted RuntimeProps. Mirrors what the bench runner does
// when materializing a cell.
func buildRuntimePropsEvidenceRecord(provider, modelRaw string, props evidence.RuntimeProps) map[string]any {
	return map[string]any{
		"schema_version": evidence.SchemaVersion,
		"record_id":      "runtime-props-import-test-" + provider,
		"captured_at":    "2026-05-14T12:00:00Z",
		"source": map[string]any{
			"type": "fizeau_runner",
			"name": "runtime_props_import_test",
		},
		"benchmark": map[string]any{
			"name":    "terminal-bench",
			"version": "2.1",
		},
		"subject": map[string]any{
			"model_raw": modelRaw,
			"harness":   "fiz",
			"provider":  provider,
		},
		"scope": map[string]any{},
		"score": map[string]any{
			"metric": "pass_rate",
			"value":  0.0,
		},
		"runtime_props": props,
	}
}
