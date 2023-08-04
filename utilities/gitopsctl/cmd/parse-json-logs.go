package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/fatih/color"
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

		parseJsonLogs()
	},
}

func parseJsonLogs() {

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

func extractAndRemoveField(fieldName string, mapVar *map[string]any) (any, bool) {

	value, exists := (*mapVar)[fieldName]

	if !exists {
		return nil, false
	}

	delete((*mapVar), fieldName)

	return value, exists

}

func extractAndRemoveStringField(fieldName string, mapVar *map[string]any) (string, bool) {

	valRes, boolRes := extractAndRemoveField(fieldName, mapVar)

	if !boolRes {
		return "", boolRes
	}

	valResStr, ok := (valRes).(string)

	if !ok {
		return "ERROR: INVALID STRING FIELD", false
	}

	return valResStr, true

}

func indentString(str string) string {

	res := ""

	for _, val := range strings.Split(strings.ReplaceAll(str, "\r\n", "\n"), "\n") {

		if len(val) == 0 {
			continue
		}

		res = "    " + val + "\n"
	}

	return res
}

func parseLine(line string) {

	jsonMap := map[string]any{}

	if err := json.Unmarshal(([]byte)(line), &jsonMap); err != nil {
		fmt.Println(err)
		return
	}

	level, exists := extractAndRemoveStringField("level", &jsonMap)
	if !exists {
		fmt.Println("Warning: level not found")
	}

	ts, exists := extractAndRemoveStringField("ts", &jsonMap)
	if !exists {
		fmt.Println("Warning: ts not found")
	} else {
		ts = strings.ReplaceAll(ts, "T", " ")
		ts = strings.ReplaceAll(ts, "Z", "")

		// Pad timestamp to 29
		for {
			if len(ts) >= 29 {
				break
			}

			ts += "0"
		}

	}

	msg, exists := extractAndRemoveStringField("msg", &jsonMap)
	if !exists {
		fmt.Println("Warning: msg not found")
	}

	namespace, namespaceExists := extractAndRemoveStringField("namespace", &jsonMap)

	workspace, workspaceExists := extractAndRemoveStringField("workspace", &jsonMap)

	stackTrace, stackTraceExists := extractAndRemoveStringField("stacktrace", &jsonMap)

	errorStr, errorExists := extractAndRemoveStringField("error", &jsonMap)

	var caller string
	var callerExists bool

	if stackTraceExists {
		caller, callerExists = extractAndRemoveStringField("caller", &jsonMap)
	}

	// color.NoColor = false

	component, componentExists := extractAndRemoveStringField("component", &jsonMap)

	red := color.New(color.FgRed).Add(color.Bold).SprintFunc()
	blue := color.New(color.FgBlue).Add(color.Bold).SprintFunc()
	cyan := color.New(color.FgCyan).Add(color.Bold).SprintFunc()
	white := color.New(color.FgHiWhite).Add(color.Bold).SprintFunc()

	green := color.New(color.FgHiGreen).Add(color.Bold).SprintFunc()

	res := fmt.Sprintf("[%s] ", white(ts))

	if level == "error" {
		res += fmt.Sprintf(" [%s] ", red("err"))
	} else {
		res += fmt.Sprintf("[%s] ", blue(level))
	}

	res += fmt.Sprintf("%s ", msg)

	// meow
	{
		section2 := ""

		if namespaceExists {
			section2 += "namespace: " + namespace + " "
		} else if workspaceExists {
			section2 += "workspace: " + workspace + " "
		}

		if componentExists {
			section2 += "component: " + component + " "
		}

		if errorExists {
			section2 += "error: " + errorStr + " "
		}

		if len(section2) > 0 {
			res += cyan("|") + " "

			res += section2
		}
	}

	res += cyan("|") + " "

	if len(jsonMap) > 0 {

		res += green("@other-fields: ")

		// Print other fields alphabetically
		keys := []string{}
		objectPresent := false

		for mapKey := range jsonMap {

			if mapKey == "object" {
				objectPresent = true
				continue
			}

			keys = append(keys, mapKey)

		}

		sort.Strings(keys)
		// The object key tends to be very large, so move it to the end
		if objectPresent {
			keys = append(keys, "object")
		}

		for _, key := range keys {

			val := jsonMap[key]

			res += fmt.Sprintf("%s:%v, ", key, val)
		}

		res = strings.TrimSuffix(res, ", ")

	}
	res = strings.TrimSpace(res)

	fmt.Println()
	fmt.Println(res)

	if stackTraceExists {
		if callerExists {
			fmt.Print(indentString("caller: " + caller))
		}

		fmt.Print(indentString("stack-trace:"))
		fmt.Print(indentString(stackTrace))
	}

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
