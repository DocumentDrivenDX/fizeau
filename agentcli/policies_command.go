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

type policyCommandRow struct {
	Name       string   `json:"name"`
	MinPower   int      `json:"min_power"`
	MaxPower   int      `json:"max_power"`
	AllowLocal bool     `json:"allow_local"`
	Require    []string `json:"require"`
}

func cmdPolicies(workDir string, jsonOut bool) int {
	svc, err := fizeau.New(fizeau.ServiceOptions{
		ServiceConfig: agentConfig.NewServiceConfig(&agentConfig.Config{}, workDir),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return 1
	}

	policies, err := svc.ListPolicies(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return 1
	}

	rows := make([]policyCommandRow, 0, len(policies))
	for _, policy := range policies {
		require := append([]string(nil), policy.Require...)
		if require == nil {
			require = []string{}
		}
		rows = append(rows, policyCommandRow{
			Name:       policy.Name,
			MinPower:   policy.MinPower,
			MaxPower:   policy.MaxPower,
			AllowLocal: policy.AllowLocal,
			Require:    require,
		})
	}

	if jsonOut {
		data, _ := json.MarshalIndent(rows, "", "  ")
		fmt.Println(string(data))
		return 0
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tMIN\tMAX\tLOCAL\tREQUIRE")
	for _, row := range rows {
		require := "-"
		if len(row.Require) > 0 {
			require = strings.Join(row.Require, ",")
		}
		fmt.Fprintf(tw, "%s\t%d\t%d\t%t\t%s\n", row.Name, row.MinPower, row.MaxPower, row.AllowLocal, require)
	}
	_ = tw.Flush()
	return 0
}
