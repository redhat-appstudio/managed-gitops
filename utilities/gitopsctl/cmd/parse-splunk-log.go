package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"

	parsejsonlogs "github.com/redhat-appstudio/managed-gitops/utilities/gitopsctl/implementations/parse-json-logs"
	"github.com/spf13/cobra"
)

// splunkLogs represents the parse splunk-logs command
var splunkLogsCmd = &cobra.Command{
	Args: func(cmd *cobra.Command, args []string) error {
		return nil
	},
	Use:   "splunk-logs",
	Short: "",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {

		fmt.Println("* Use CTRL-C to exit.")

		ParseSplunkLogs()
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

func ParseSplunkLogs() {

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Text()
		parseLine(line)
	}

	if scanner.Err() != nil {
		fmt.Println(scanner.Err().Error())
		os.Exit(1)
		return
	}
}

func parseLine(line string) {

	// fmt.Println("------------------")

	jsonMap := map[string]any{}

	if err := json.Unmarshal(([]byte)(line), &jsonMap); err != nil {
		fmt.Println(err)
		return
	}

	var structuredMap map[string]any

	structuredMapVal, exists := jsonMap["structured"]
	if exists {

		structuredMap = (structuredMapVal).(map[string]any)

		delete(jsonMap, "structured")
	}

	parsejsonlogs.ParseMap(structuredMap, jsonMap)

	// for k, v := range jsonMap {
	// 	fmt.Println(k, "->", v)
	// }

	// fmt.Println(line)
}

func init() {
	parseCmd.AddCommand(splunkLogsCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// jobCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// jobCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
