package agentcli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/DocumentDrivenDX/fizeau"
	agentConfig "github.com/DocumentDrivenDX/fizeau/internal/config"
)

type harnessCommandRow struct {
	Name                 string   `json:"name"`
	Type                 string   `json:"type"`
	Available            bool     `json:"available"`
	Path                 string   `json:"path,omitempty"`
	Error                string   `json:"error,omitempty"`
	Billing              string   `json:"billing"`
	AutoRoutingEligible  bool     `json:"auto_routing_eligible"`
	TestOnly             bool     `json:"test_only"`
	ExactPinSupport      bool     `json:"exact_pin_support"`
	DefaultModel         string   `json:"default_model,omitempty"`
	CostClass            string   `json:"cost_class,omitempty"`
	SupportedReasoning   []string `json:"supported_reasoning,omitempty"`
	SupportedPermissions []string `json:"supported_permissions,omitempty"`
}

func cmdHarnesses(workDir string, jsonOut bool) int {
	svc, err := fizeau.New(fizeau.ServiceOptions{
		ServiceConfig: agentConfig.NewServiceConfig(&agentConfig.Config{}, workDir),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return 1
	}

	harnesses, err := svc.ListHarnesses(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return 1
	}

	rows := make([]harnessCommandRow, 0, len(harnesses))
	for _, harness := range harnesses {
		rows = append(rows, harnessCommandRow{
			Name:                 harness.Name,
			Type:                 harness.Type,
			Available:            harness.Available,
			Path:                 harness.Path,
			Error:                harness.Error,
			Billing:              billingLabel(harness.Billing),
			AutoRoutingEligible:  harness.AutoRoutingEligible,
			TestOnly:             harness.TestOnly,
			ExactPinSupport:      harness.ExactPinSupport,
			DefaultModel:         harness.DefaultModel,
			CostClass:            harness.CostClass,
			SupportedReasoning:   append([]string(nil), harness.SupportedReasoning...),
			SupportedPermissions: append([]string(nil), harness.SupportedPermissions...),
		})
	}

	if jsonOut {
		data, _ := json.MarshalIndent(rows, "", "  ")
		fmt.Println(string(data))
		return 0
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tTYPE\tBILLING\tSTATUS\tMODEL\tROUTE")
	for _, row := range rows {
		status := "unavailable"
		if row.Available {
			status = "available"
		}
		if row.Error != "" {
			status = row.Error
		}
		model := row.DefaultModel
		if model == "" {
			model = "-"
		}
		route := "no"
		if row.AutoRoutingEligible {
			route = "yes"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", row.Name, row.Type, row.Billing, compactTableValue(status, 36), model, route)
	}
	_ = tw.Flush()
	return 0
}

func billingLabel(model fizeau.BillingModel) string {
	if model == "" {
		return "unknown"
	}
	return string(model)
}

func compactTableValue(value string, limit int) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) <= limit {
		return value
	}
	if limit <= 2 {
		return value[:limit]
	}
	return value[:limit-2] + ".."
}
