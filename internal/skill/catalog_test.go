package skill

import (
	"os"
	"path/filepath"
	"testing"
)

func writeSkill(t *testing.T, dir, rel, content string) string {
	t.Helper()
	full := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	return full
}

func TestScanDir_NonExistent(t *testing.T) {
	cat, warns, err := ScanDir(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if cat == nil {
		t.Fatal("catalog is nil")
	}
	if cat.Len() != 0 {
		t.Errorf("len = %d, want 0", cat.Len())
	}
	if len(warns) != 0 {
		t.Errorf("warnings = %v, want none", warns)
	}
}

func TestScanDir_EmptyDir(t *testing.T) {
	cat, _, err := ScanDir(t.TempDir())
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if cat.Len() != 0 {
		t.Errorf("len = %d", cat.Len())
	}
}

func TestScanDir_EmptyPath(t *testing.T) {
	cat, _, err := ScanDir("")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if cat.Len() != 0 {
		t.Errorf("len = %d", cat.Len())
	}
}

func TestScanDir_FileNotDir(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "afile")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err := ScanDir(f)
	if err == nil {
		t.Fatal("expected error for non-directory path")
	}
}

func TestScanDir_MixOfValidAndInvalid(t *testing.T) {
	tmp := t.TempDir()
	writeSkill(t, tmp, "fix-tests/SKILL.md",
		"---\nname: fix-tests\ndescription: Fix tests.\n---\nBODY-A\n")
	writeSkill(t, tmp, "scaffold/SKILL.md",
		"---\nname: scaffold\ndescription: Scaffold a service.\n---\nBODY-B\n")
	// Invalid: missing description.
	writeSkill(t, tmp, "broken/SKILL.md",
		"---\nname: broken\n---\nbody\n")
	// Not a SKILL.md file.
	writeSkill(t, tmp, "notes/README.md", "# notes\n")

	cat, warns, err := ScanDir(tmp)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if cat.Len() != 2 {
		t.Errorf("len = %d, want 2", cat.Len())
	}
	if cat.ByName("fix-tests") == nil {
		t.Error("fix-tests missing")
	}
	if cat.ByName("scaffold") == nil {
		t.Error("scaffold missing")
	}
	if cat.ByName("broken") != nil {
		t.Error("broken should be skipped")
	}
	if len(warns) == 0 {
		t.Error("expected warning for broken skill")
	}
	// Names are returned sorted.
	names := cat.Names()
	if len(names) != 2 || names[0] != "fix-tests" || names[1] != "scaffold" {
		t.Errorf("names = %v, want [fix-tests scaffold]", names)
	}
}

func TestCatalog_LoadBody(t *testing.T) {
	tmp := t.TempDir()
	const body = "# Heading\n\nDo the thing.\n"
	writeSkill(t, tmp, "x/SKILL.md", "---\nname: x\ndescription: y\n---\n"+body)

	cat, _, err := ScanDir(tmp)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	got, err := cat.LoadBody("x")
	if err != nil {
		t.Fatalf("LoadBody: %v", err)
	}
	if got != body {
		t.Errorf("body = %q, want %q", got, body)
	}
}

func TestCatalog_LoadBody_Unknown(t *testing.T) {
	cat, _, _ := ScanDir(t.TempDir())
	_, err := cat.LoadBody("nope")
	if err == nil {
		t.Fatal("expected error for unknown skill")
	}
}

func TestCatalog_ByName_NilSafe(t *testing.T) {
	var cat *Catalog
	if cat.ByName("anything") != nil {
		t.Error("nil catalog ByName should return nil")
	}
	if cat.Len() != 0 {
		t.Error("nil catalog Len should return 0")
	}
	if cat.Names() != nil {
		t.Error("nil catalog Names should return nil")
	}
}

func TestCatalog_DuplicateNames(t *testing.T) {
	tmp := t.TempDir()
	writeSkill(t, tmp, "a/SKILL.md", "---\nname: dup\ndescription: A\n---\nA\n")
	writeSkill(t, tmp, "b/SKILL.md", "---\nname: dup\ndescription: B\n---\nB\n")

	cat, _, err := ScanDir(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if cat.Len() != 1 {
		t.Errorf("len = %d, want 1 (dedup by name)", cat.Len())
	}
}

func TestScanDir_NestedDirectories(t *testing.T) {
	tmp := t.TempDir()
	writeSkill(t, tmp, "deeply/nested/path/SKILL.md",
		"---\nname: deep\ndescription: D\n---\nbody\n")
	cat, _, err := ScanDir(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if cat.ByName("deep") == nil {
		t.Error("nested skill not discovered")
	}
}
