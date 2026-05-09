package targetlint

import (
	"strings"
	"testing"
)

func TestFlagsTargetInRoutingContext(t *testing.T) {
	content := strings.Join([]string{
		"package fizeau",
		"func policyAlias(req request) string {",
		"	target := req.Policy",
		"	return target",
		"}",
		"// target-level routing is legacy vocabulary.",
	}, "\n") + "\n"

	findings := ScanContent("service_policies.go", []byte(content))
	if len(findings) != 3 {
		t.Fatalf("findings = %#v, want 3", findings)
	}
}

func TestAllowsHealthTargetAndErrorsIsTarget(t *testing.T) {
	content := strings.Join([]string{
		"package fizeau",
		"type HealthTarget struct{}",
		"func (s service) HealthCheck(target HealthTarget) error {",
		"	return nil",
		"}",
	}, "\n") + "\n"
	if findings := ScanContent("service_providers.go", []byte(content)); len(findings) != 0 {
		t.Fatalf("HealthTarget findings = %#v, want none", findings)
	}

	errorsContent := strings.Join([]string{
		"package routing",
		"func (e errType) Is(target error) bool {",
		"	switch target.(type) {",
		"	default:",
		"		return errors.Is(sentinel, target)",
		"	}",
		"}",
	}, "\n") + "\n"
	if findings := ScanContent("internal/routing/errors.go", []byte(errorsContent)); len(findings) != 0 {
		t.Fatalf("errors.Is findings = %#v, want none", findings)
	}
}

func TestRepositoryTargetVocabulary(t *testing.T) {
	findings, err := Scan(Options{Root: "../../.."})
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("unexpected target vocabulary findings: %#v", findings)
	}
}
