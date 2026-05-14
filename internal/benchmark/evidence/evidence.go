package evidence

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

const (
	SchemaRelativePath = "scripts/benchmark/benchmark-evidence.schema.json"
	SchemaVersion      = "benchmark-evidence/v1"
)

// SamplingDefaults holds server-reported default sampling parameters.
type SamplingDefaults struct {
	Temperature   *float64 `json:"temperature,omitempty"`
	TopP          *float64 `json:"top_p,omitempty"`
	TopK          *int     `json:"top_k,omitempty"`
	RepeatPenalty *float64 `json:"repeat_penalty,omitempty"`
}

// RuntimeProps holds per-cell server-reported properties captured before the
// bench run. All fields are optional; platforms report different subsets.
// When extraction fails the Extractor and ExtractionFailed fields are set and
// the rest is zero-valued.
type RuntimeProps struct {
	Extractor         string            `json:"extractor,omitempty"`
	ExtractedAt       *time.Time        `json:"extracted_at,omitempty"`
	ExtractionFailed  string            `json:"extraction_failed,omitempty"`
	BaseModel         string            `json:"base_model,omitempty"`
	ModelQuant        string            `json:"model_quant,omitempty"`
	KVQuant           string            `json:"kv_quant,omitempty"`
	DraftModel        string            `json:"draft_model,omitempty"`
	DraftMode         string            `json:"draft_mode,omitempty"`
	MaxContext        *int              `json:"max_context,omitempty"`
	GPULayers         *int              `json:"gpu_layers,omitempty"`
	MTPEnabled        *bool             `json:"mtp_enabled,omitempty"`
	SpeculativeN      *int              `json:"speculative_n,omitempty"`
	ServerVersion     string            `json:"server_version,omitempty"`
	BuildInfo         string            `json:"build_info,omitempty"`
	SamplingDefaults  *SamplingDefaults `json:"sampling_defaults,omitempty"`
	PlatformRaw       map[string]any    `json:"platform_raw,omitempty"`
}

// Validator loads and applies the benchmark evidence schema from a repo root.
type Validator struct {
	schema *jsonschema.Schema
}

// AppendReport summarizes the effect of an append operation.
type AppendReport struct {
	LedgerPath string
	Added      int
	Duplicates []string
}

// DuplicateRecordsError reports duplicate evidence records that were skipped.
type DuplicateRecordsError struct {
	RecordIDs []string
}

func (e *DuplicateRecordsError) Error() string {
	if e == nil || len(e.RecordIDs) == 0 {
		return "duplicate evidence records"
	}
	return fmt.Sprintf("duplicate evidence records: %s", strings.Join(e.RecordIDs, ", "))
}

// NewValidator compiles the benchmark evidence schema from repoRoot.
func NewValidator(repoRoot string) (*Validator, error) {
	schemaPath := filepath.Join(repoRoot, SchemaRelativePath)
	rawSchema, err := os.ReadFile(schemaPath) // #nosec G304 -- schemaPath joins caller-supplied repo root with constant relative path
	if err != nil {
		return nil, fmt.Errorf("read evidence schema %s: %w", schemaPath, err)
	}

	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat = true
	if err := compiler.AddResource("benchmark-evidence.schema.json", bytes.NewReader(rawSchema)); err != nil {
		return nil, fmt.Errorf("add evidence schema: %w", err)
	}
	schema, err := compiler.Compile("benchmark-evidence.schema.json")
	if err != nil {
		return nil, fmt.Errorf("compile evidence schema: %w", err)
	}
	return &Validator{schema: schema}, nil
}

// ValidateFile validates one JSON or JSONL evidence file and returns the
// normalized records, including any generated record_id values.
func (v *Validator) ValidateFile(path string) ([]map[string]any, error) {
	rawRecords, err := loadRecords(path)
	if err != nil {
		return nil, err
	}

	normalized := make([]map[string]any, 0, len(rawRecords))
	for i, record := range rawRecords {
		doc, _, err := v.normalizeRecord(record)
		if err != nil {
			return nil, fmt.Errorf("%s line %d: %w", path, i+1, err)
		}
		normalized = append(normalized, doc)
	}
	return normalized, nil
}

// AppendLedger appends validated evidence records to a JSONL ledger.
func (v *Validator) AppendLedger(inputPath, ledgerPath string) (*AppendReport, error) {
	incoming, err := v.ValidateFile(inputPath)
	if err != nil {
		return nil, err
	}

	existing, err := loadLedgerRecords(ledgerPath)
	if err != nil {
		return nil, err
	}

	existingByFingerprint := make(map[string]string, len(existing))
	existingByID := make(map[string]string, len(existing))
	for _, record := range existing {
		fp, err := fingerprintRecord(record)
		if err != nil {
			return nil, err
		}
		id, _ := record["record_id"].(string)
		existingByFingerprint[fp] = id
		existingByID[id] = fp
	}

	var (
		additions  []map[string]any
		duplicates []string
	)
	seenNew := make(map[string]struct{}, len(incoming))
	for _, record := range incoming {
		fp, err := fingerprintRecord(record)
		if err != nil {
			return nil, err
		}
		id, _ := record["record_id"].(string)

		if existingID, ok := existingByFingerprint[fp]; ok {
			duplicates = append(duplicates, chooseDuplicateID(id, existingID))
			continue
		}
		if existingFP, ok := existingByID[id]; ok && existingFP != fp {
			return nil, fmt.Errorf("record_id %q already exists with different content", id)
		}
		if _, ok := seenNew[fp]; ok {
			duplicates = append(duplicates, id)
			continue
		}

		seenNew[fp] = struct{}{}
		additions = append(additions, record)
		existingByFingerprint[fp] = id
		existingByID[id] = fp
	}

	if len(additions) > 0 {
		if err := appendRecords(ledgerPath, additions); err != nil {
			return nil, err
		}
	}

	report := &AppendReport{
		LedgerPath: ledgerPath,
		Added:      len(additions),
		Duplicates: append([]string(nil), duplicates...),
	}
	if len(duplicates) > 0 {
		return report, &DuplicateRecordsError{RecordIDs: duplicates}
	}
	return report, nil
}

// StableRecordID returns the deterministic content-derived record_id for a
// record that omits record_id.
func StableRecordID(doc map[string]any) (string, error) {
	normalized, err := cloneDocument(doc)
	if err != nil {
		return "", err
	}
	delete(normalized, "record_id")
	sum := sha256.Sum256(mustMarshal(normalized))
	return hex.EncodeToString(sum[:]), nil
}

func (v *Validator) normalizeRecord(doc map[string]any) (map[string]any, string, error) {
	normalized, err := cloneDocument(doc)
	if err != nil {
		return nil, "", err
	}
	fp, err := fingerprintRecord(normalized)
	if err != nil {
		return nil, "", err
	}
	if id, _ := normalized["record_id"].(string); strings.TrimSpace(id) == "" {
		normalized["record_id"] = fp
	}
	if err := v.schema.Validate(normalized); err != nil {
		return nil, "", fmt.Errorf("schema validation failed: %w", err)
	}
	return normalized, fp, nil
}

func fingerprintRecord(doc map[string]any) (string, error) {
	normalized, err := cloneDocument(doc)
	if err != nil {
		return "", err
	}
	delete(normalized, "record_id")
	sum := sha256.Sum256(mustMarshal(normalized))
	return hex.EncodeToString(sum[:]), nil
}

func loadLedgerRecords(path string) ([]map[string]any, error) {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("stat ledger %s: %w", path, err)
	}
	if info.Size() == 0 {
		return nil, nil
	}
	return loadRecords(path)
}

func loadRecords(path string) ([]map[string]any, error) {
	raw, err := os.ReadFile(path) // #nosec G304 -- path is operator-supplied evidence file
	if err != nil {
		return nil, fmt.Errorf("read evidence file %s: %w", path, err)
	}

	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("evidence file %s is empty", path)
	}

	if record, err := decodeObject(trimmed); err == nil {
		return []map[string]any{record}, nil
	}

	lines := bytes.Split(trimmed, []byte("\n"))
	records := make([]map[string]any, 0, len(lines))
	for i, line := range lines {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		record, err := decodeObject(line)
		if err != nil {
			return nil, fmt.Errorf("%s line %d: %w", path, i+1, err)
		}
		records = append(records, record)
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("evidence file %s contains no records", path)
	}
	return records, nil
}

func decodeObject(raw []byte) (map[string]any, error) {
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	return doc, nil
}

func cloneDocument(doc map[string]any) (map[string]any, error) {
	raw, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("marshal evidence record: %w", err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("clone evidence record: %w", err)
	}
	return out, nil
}

func mustMarshal(doc map[string]any) []byte {
	raw, err := json.Marshal(doc)
	if err != nil {
		panic(err)
	}
	return raw
}

func appendRecords(path string, records []map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { // #nosec G301 -- ledger dir must be group-readable for ops tooling
		return fmt.Errorf("create ledger directory: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600) // #nosec G304 -- path is operator-supplied ledger file
	if err != nil {
		return fmt.Errorf("open ledger %s: %w", path, err)
	}
	defer f.Close()

	for _, record := range records {
		raw, err := json.Marshal(record)
		if err != nil {
			return fmt.Errorf("marshal evidence record: %w", err)
		}
		if _, err := f.Write(append(raw, '\n')); err != nil {
			return fmt.Errorf("write ledger %s: %w", path, err)
		}
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("sync ledger %s: %w", path, err)
	}
	return nil
}

func chooseDuplicateID(preferred, fallback string) string {
	if strings.TrimSpace(preferred) != "" {
		return preferred
	}
	return fallback
}
