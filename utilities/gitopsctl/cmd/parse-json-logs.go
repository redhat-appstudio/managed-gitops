package cmd

import (
	"fmt"

	parsejsonlogs "github.com/redhat-appstudio/managed-gitops/utilities/gitopsctl/implementations/parse-json-logs"
	"github.com/spf13/cobra"
)

// jsonLogs represents the parse json-logs command
var jsonLogsCmd = &cobra.Command{
	Args: func(cmd *cobra.Command, args []string) error {
		return nil
	},
	Use:   "json-logs",
	Short: "Parse JSON logs into a developer friendly format, from Goreman/K8s Pods/Splunk raw",
	Long: `
The 'json-logs' command will parse a JSON-formatted log file/stream into a more 
user/developer friendly format. This command is primarily specialized to 
parsing GitOps Service controller logs.

Features:
- Support for both JSON from Pod logs, JSON from goreman, and JSON from splunk 'raw' logs
- Colour coded fields to make it easy to find data
- Important fields are prioritized, less-import fields are deprioritized
- Fields that are known to be useless are removed (especially for splunk fields)

Examples:
		
- Parse an existing log file:
	cat (log file) | gitopsctl parse json-logs

- Parse output from a running Goreman dev environment:
	make start-e2e | utilities/gitopsctl/gitopsctl parse json-logs

- Modify the log stream before parsing it, such as sorting or removing some output:
	cat (log file) | sort | grep -v "info" | grep -v "some other message" | gitopsctl parse json-logs
`,
	Run: func(cmd *cobra.Command, args []string) {

		fmt.Println("* Use CTRL-C to exit.")

		parsejsonlogs.ParseJsonLogsFromStdin()
	},
}

func init() {
	parseCmd.AddCommand(jsonLogsCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// jobCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// jobCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
