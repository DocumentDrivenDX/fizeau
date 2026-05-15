package fizeau

import (
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"strings"
	"testing"
)

// structural test that the legacy provider-reliability metric is surfaced as
// a separate field (not folded into RoutingQuality). Parses service.go
// directly so the assertion is robust to docstring drift.
func TestProviderReliabilityNotRenamedToRoutingQuality(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "service.go", nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse service.go: %v", err)
	}

	candidateFields := findStructFields(file, "RouteCandidateStatus")
	if candidateFields == nil {
		t.Fatalf("RouteCandidateStatus struct not found")
	}
	if _, ok := candidateFields["ProviderReliabilityRate"]; !ok {
		t.Fatalf("RouteCandidateStatus.ProviderReliabilityRate missing; available fields: %v", candidateFields)
	}
	if got := candidateFields["ProviderReliabilityRate"]; got != "float64" {
		t.Errorf("ProviderReliabilityRate type = %s, want float64", got)
	}

	reportFields := findStructFields(file, "RouteStatusReport")
	if reportFields == nil {
		t.Fatalf("RouteStatusReport struct not found")
	}
	got, ok := reportFields["RoutingQuality"]
	if !ok {
		t.Fatalf("RouteStatusReport.RoutingQuality missing; available: %v", reportFields)
	}
	if got != "RoutingQualityMetrics" {
		t.Errorf("RouteStatusReport.RoutingQuality type = %s, want RoutingQualityMetrics", got)
	}
	for name := range reportFields {
		lower := strings.ToLower(name)
		if strings.Contains(lower, "providerreliability") || strings.Contains(lower, "successrate") {
			t.Errorf("RouteStatusReport unexpectedly carries provider-reliability field %q (should live on RouteCandidateStatus)", name)
		}
	}
}

func TestRoutingQualityFieldsExposedOnPublicTypes(t *testing.T) {
	report := reflect.TypeOf(RouteStatusReport{})
	usage := reflect.TypeOf(UsageReport{})
	rq := reflect.TypeOf(RoutingQualityMetrics{})
	bucket := reflect.TypeOf(OverrideClassBucket{})

	assertFieldTypeAndTag(t, report, "RoutingQuality", "RoutingQualityMetrics", "")
	assertFieldTypeAndTag(t, usage, "RoutingQuality", "RoutingQualityMetrics", `json:"routing_quality"`)

	assertFieldTypeAndTag(t, rq, "AutoAcceptanceRate", "float64", `json:"auto_acceptance_rate"`)
	assertFieldTypeAndTag(t, rq, "OverrideDisagreementRate", "float64", `json:"override_disagreement_rate"`)
	assertFieldTypeAndTag(t, rq, "OverrideClassBreakdown", "[]OverrideClassBucket", `json:"override_class_breakdown,omitempty"`)
	assertFieldTypeAndTag(t, rq, "TotalRequests", "int", `json:"total_requests"`)
	assertFieldTypeAndTag(t, rq, "TotalOverrides", "int", `json:"total_overrides"`)

	assertFieldTypeAndTag(t, bucket, "PromptFeatureBucket", "string", `json:"prompt_feature_bucket"`)
	assertFieldTypeAndTag(t, bucket, "Axis", "string", `json:"axis"`)
	assertFieldTypeAndTag(t, bucket, "Match", "bool", `json:"match"`)
	assertFieldTypeAndTag(t, bucket, "Count", "int", `json:"count"`)
	assertFieldTypeAndTag(t, bucket, "SuccessOutcomes", "int", `json:"success_outcomes"`)
	assertFieldTypeAndTag(t, bucket, "StalledOutcomes", "int", `json:"stalled_outcomes"`)
	assertFieldTypeAndTag(t, bucket, "FailedOutcomes", "int", `json:"failed_outcomes"`)
	assertFieldTypeAndTag(t, bucket, "CancelledOutcomes", "int", `json:"cancelled_outcomes"`)
	assertFieldTypeAndTag(t, bucket, "UnknownOutcomes", "int", `json:"unknown_outcomes"`)
}

func assertFieldTypeAndTag(t *testing.T, typ reflect.Type, fieldName, wantType, wantTag string) {
	t.Helper()
	field, ok := typ.FieldByName(fieldName)
	if !ok {
		t.Fatalf("%s missing field %s", typ.Name(), fieldName)
	}
	gotType := strings.ReplaceAll(field.Type.String(), "fizeau.", "")
	if gotType != wantType {
		t.Errorf("%s.%s type = %s, want %s", typ.Name(), fieldName, gotType, wantType)
	}
	if got := string(field.Tag); got != wantTag {
		t.Errorf("%s.%s tag = %q, want %q", typ.Name(), fieldName, got, wantTag)
	}
}

// findStructFields parses an *ast.File and returns a map of field name →
// rendered field type for the named struct, or nil if not found.
func findStructFields(file *ast.File, name string) map[string]string {
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}
		for _, spec := range gen.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok || ts.Name.Name != name {
				continue
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok || st.Fields == nil {
				continue
			}
			out := make(map[string]string)
			for _, f := range st.Fields.List {
				typeStr := exprString(f.Type)
				for _, n := range f.Names {
					out[n.Name] = typeStr
				}
			}
			return out
		}
	}
	return nil
}

func exprString(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.SelectorExpr:
		return exprString(v.X) + "." + v.Sel.Name
	case *ast.StarExpr:
		return "*" + exprString(v.X)
	case *ast.ArrayType:
		return "[]" + exprString(v.Elt)
	case *ast.MapType:
		return "map[" + exprString(v.Key) + "]" + exprString(v.Value)
	default:
		return ""
	}
}
