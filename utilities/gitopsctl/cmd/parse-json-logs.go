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
	Short: "",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {

		fmt.Println("* Use CTRL-C to exit.")

		parsejsonlogs.ParseJsonLogs()
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
