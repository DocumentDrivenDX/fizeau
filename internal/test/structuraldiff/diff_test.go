package structuraldiff

import "testing"

func TestCompareJSON_AllowsRFC3339TimeVariance(t *testing.T) {
	want := []byte(`{"CapturedAt":"2026-05-14T17:00:00Z"}`)
	got := []byte(`{"CapturedAt":"2026-05-15T01:02:03.123456789Z"}`)
	if err := CompareJSON(want, got, Config{}); err != nil {
		t.Fatalf("CompareJSON() error = %v, want nil", err)
	}
}

func TestCompareJSON_RejectsInvalidTimeField(t *testing.T) {
	want := []byte(`{"captured_at":"2026-05-14T17:00:00Z"}`)
	got := []byte(`{"captured_at":"not-a-time"}`)
	if err := CompareJSON(want, got, Config{}); err == nil {
		t.Fatal("CompareJSON() error = nil, want invalid time error")
	}
}

func TestCompareJSON_AllowsDocumentedAdditiveField(t *testing.T) {
	want := []byte(`{"Quota":{"status":"ok"}}`)
	got := []byte(`{"Quota":{"status":"ok","new_field":"documented"}}`)
	err := CompareJSON(want, got, Config{AdditivePaths: []string{"Quota.new_field"}})
	if err != nil {
		t.Fatalf("CompareJSON() error = %v, want nil", err)
	}
}

func TestCompareJSON_PresenceOnlyPathIgnoresValueDrift(t *testing.T) {
	want := []byte(`{"opaque":{"detail":"baseline"}}`)
	got := []byte(`{"opaque":{"detail":"changed","extra":"field"}}`)
	err := CompareJSON(want, got, Config{PresenceOnlyPaths: []string{"opaque"}})
	if err != nil {
		t.Fatalf("CompareJSON() error = %v, want nil", err)
	}
}
