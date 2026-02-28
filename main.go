package main

import (
	"fmt"
	"os"
	"runtime/debug"

	"github.com/spf13/cobra"
)

var version = "dev"

func getVersion() string {
	if version != "dev" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return version
}

var rootCmd = &cobra.Command{
	Use:   "frm",
	Short: "Maintain meaningful relationships with a CardDAV-backed contact tracker",
	Long: `frm is a CLI friend relationship manager backed by CardDAV.
Your contacts live in your CardDAV server, interaction history in ~/.frm/log.jsonl.

Getting Started:
  frm init                          Set up your CardDAV connection
  frm triage                        Categorize contacts (monthly, quarterly, yearly, or custom like 2w)
  frm check                         See who's overdue for a catch-up
  frm log "Name" --note "had coffee" Record an interaction

For Automation:
  frm triage --json                 Get untriaged contacts as JSON for agent-driven workflows
  frm track "Name" --every 2w       Set contact frequency (use with triage --json)
  frm ignore "Name"                 Hide a contact from triage
  --json                            Available on all commands for structured output
  --dry-run                         Preview changes without writing anything`,
	Version: getVersion(),
}

func init() {
	rootCmd.PersistentFlags().Bool("dry-run", false, "Show what would happen without making changes")

	rootCmd.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print the version number",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("frm " + getVersion())
		},
	})
}

// isDryRun returns true if the --dry-run flag is set on the command.
func isDryRun(cmd *cobra.Command) bool {
	v, _ := cmd.Flags().GetBool("dry-run")
	return v
}

func main() {
	// Silence cobra's default error printing so we can handle it ourselves.
	rootCmd.SilenceErrors = true

	if err := rootCmd.Execute(); err != nil {
		// If --json was set on the command that failed, output structured JSON error.
		if jsonFlag, _ := rootCmd.PersistentFlags().GetBool("json"); jsonFlag {
			printJSONError(rootCmd, err)
		} else {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(1)
	}
}
