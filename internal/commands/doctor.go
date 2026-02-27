package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

// DoctorCmd returns the "doctor" command for health checks.
func DoctorCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:     "doctor",
		Short:   "Health checks",
		Long:    "Run diagnostic checks on the ComputeCommander installation and its dependencies.",
		GroupID: "INFRASTRUCTURE",
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonOut, _ := cmd.Root().Flags().GetBool("json")

			checks := runDoctorChecks(app)

			if jsonOut {
				return json.NewEncoder(os.Stdout).Encode(checks)
			}

			allOK := true
			for _, c := range checks {
				status := "OK"
				if !c.OK {
					status = "FAIL"
					allOK = false
				}
				fmt.Printf("  [%4s] %s: %s\n", status, c.Name, c.Detail)
			}

			if allOK {
				fmt.Println("\nAll checks passed.")
			} else {
				fmt.Println("\nSome checks failed. Please review the output above.")
			}
			return nil
		},
	}
}

type doctorCheck struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}

func runDoctorChecks(app *App) []doctorCheck {
	var checks []doctorCheck

	// Config check.
	if app.Config != nil {
		if err := app.Config.Validate(); err != nil {
			checks = append(checks, doctorCheck{"config", false, err.Error()})
		} else {
			checks = append(checks, doctorCheck{"config", true, "valid"})
		}
	} else {
		checks = append(checks, doctorCheck{"config", false, "not loaded"})
	}

	// Database check.
	if app.DB != nil {
		checks = append(checks, doctorCheck{"database", true, fmt.Sprintf("driver=%s", app.DB.Driver())})
	} else {
		checks = append(checks, doctorCheck{"database", false, "not connected"})
	}

	// Git check.
	if _, err := exec.LookPath("git"); err != nil {
		checks = append(checks, doctorCheck{"git", false, "git not found in PATH"})
	} else {
		checks = append(checks, doctorCheck{"git", true, "available"})
	}

	// Zellij check.
	if _, err := exec.LookPath("zellij"); err != nil {
		checks = append(checks, doctorCheck{"zellij", false, "zellij not found in PATH"})
	} else {
		checks = append(checks, doctorCheck{"zellij", true, "available"})
	}

	// Project directory check.
	if _, err := os.Stat(".computecommander"); err != nil {
		checks = append(checks, doctorCheck{"project", false, ".computecommander/ not found"})
	} else {
		checks = append(checks, doctorCheck{"project", true, ".computecommander/ exists"})
	}

	return checks
}
