package parsejsonlogs

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/fatih/color"
)

// Feedback welcome if these colors look bad on other terminals/OSes
var (
	timestampColor           = color.New(color.FgHiWhite).Add(color.Bold).SprintFunc()
	infoColor                = color.New(color.FgBlue).Add(color.Bold).SprintFunc()
	errorColor               = color.New(color.FgRed).Add(color.Bold).SprintFunc()
	separatorColor           = color.New(color.FgCyan).Add(color.Bold).SprintFunc()
	fieldColor               = color.New(color.FgGreen).SprintFunc()
	splunkSeparatorTextColor = color.New(color.FgYellow).Add(color.Bold).SprintFunc()
	separatorTextColor       = color.New(color.FgMagenta).Add(color.Bold).SprintFunc()
)

func ParseJsonLogsFromStdin() {

	lineCount := 1

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Text()

		parseJSONLogLine(line, lineCount)

		lineCount++
	}

	if scanner.Err() != nil {
		fmt.Println(scanner.Err().Error())
		os.Exit(1)
		return
	}
}

func ReadAllLinesFirstThenSortByTimestamp() {

	lines := []string{}

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Text()

		lines = append(lines, line)
	}

	if scanner.Err() != nil {
		fmt.Println(scanner.Err().Error())
		os.Exit(1)
		return
	}

	// Extract timestamp and sort by that
	sort.Sort(ByTS(lines))

	// Parse in sorted order
	for _, line := range lines {
		parseJSONLogLine(line, 0)
	}

}

func parseJSONLogLine(line string, lineNumber int) {

	// A) If the log line is from a Goreman log, pre-format it to make it parseable by JSON
	isGoreman, jsonModified, line, beforeJsonLocalDevPrefix := parseGoremanLogsIfApplicable(line)

	if isGoreman && !jsonModified {
		// We weren't able to extract any JSON, so just print the line as is
		fmt.Println(line)
		return
	}

	fullJsonMap := map[string]any{}

	if err := json.Unmarshal(([]byte)(line), &fullJsonMap); err != nil {
		// We weren't able to extract any JSON, so just print the line as is
		fmt.Println(line)
		return
	}

	var structuredMap map[string]any

	var output string

	structuredMapVal, exists := fullJsonMap["structured"]
	if exists {

		// B) If the 'structured' field exists, then the log is from splunk

		structuredMap = (structuredMapVal).(map[string]any)

		delete(fullJsonMap, "structured")

		output = parseJSONMapFromLine(structuredMap, fullJsonMap, lineNumber)
	} else {

		_, hasTimestampField := fullJsonMap["@timestamp"]

		if hasTimestampField {

			// C) If the 'structured' field doesn't exist and @timestamp DOES exist, then the log is direct splunk output
			output = parseJSONMapFromLine(make(map[string]any), fullJsonMap, lineNumber)

		} else {
			// D) If the 'structured' field doesn't exist and @timestamp doesn't exist, then the log is direct controller output (e.g. not from splunk)
			output = parseJSONMapFromLine(fullJsonMap, make(map[string]any), lineNumber)
		}

	}

	// Output the line (optionally with prefix for the Goreman case, only)

	fmt.Println(beforeJsonLocalDevPrefix + output)
	fmt.Println()

}

// The Goreman log lines need to be carefully cut up to allow us to both parse the JSON, but also preserve the Goreman color prefix.
func parseGoremanLogsIfApplicable(line string) (bool, bool, string, string) {
	var beforeJsonLocalDevPrefix string

	isGoremanLocalDev := false // true if the logs are from goreman, false otherwise

	goremanPrefixes := []string{
		" backend | ", " appstudio-controller | ", " cluster-agent | ",
	}

	for _, goremanPrefix := range goremanPrefixes {

		if strings.Contains(line, goremanPrefix) {
			isGoremanLocalDev = true
			break
		}
	}

	jsonModified := false // true if there is json on the line that we were able to extract, false otherwise. If false this implies it is not a JSON log line

	if isGoremanLocalDev {
		firstPipeIndex := strings.Index(line, "|")
		if firstPipeIndex != -1 {
			afterPipe := line[firstPipeIndex+1:]

			firstCurlyBraceIndex := strings.Index(afterPipe, "{")
			if firstCurlyBraceIndex != -1 {

				beforeJsonLocalDevPrefix = line[0:strings.Index(line, "{")]

				// Remove all double spaces from the text before the first '{' character.
				for {

					if !strings.Contains(beforeJsonLocalDevPrefix, "  ") {
						break
					}

					beforeJsonLocalDevPrefix = strings.ReplaceAll(beforeJsonLocalDevPrefix, "  ", " ")
				}

				// Remove the time, and whitespace, from the line, leaving the color and goreman process prefix
				{
					upToAndIncludingFirstMChar := beforeJsonLocalDevPrefix[0 : strings.Index(beforeJsonLocalDevPrefix, "m")+1]
					everythingAfterFirstSpace := beforeJsonLocalDevPrefix[strings.Index(beforeJsonLocalDevPrefix, " ")+1:]

					beforeJsonLocalDevPrefix = upToAndIncludingFirstMChar + everythingAfterFirstSpace
				}

				line = afterPipe[firstCurlyBraceIndex:] // update line to be only the JSON string, so that it can be parsed
				jsonModified = true

			}
		}
	}

	return isGoremanLocalDev, jsonModified, line, beforeJsonLocalDevPrefix

}

func parseJSONMapFromLine(structuredJsonMap map[string]any, splunkJsonMap map[string]any, lineNumber int) string {

	// Add the line number of the origin log file as field, if available
	// - this lets users easily find the original value in the original log, by searching for the line number
	if lineNumber > 0 {
		if len(structuredJsonMap) != 0 {
			structuredJsonMap["logLineNumber"] = fmt.Sprintf("%v", lineNumber)
		} else if len(splunkJsonMap) != 0 {
			splunkJsonMap["logLineNumber"] = fmt.Sprintf("%v", lineNumber)
		}
	}

	// The splunk logs contain a lot of useless fields, so filter them out
	splunkJsonMap = filterByMapKey(splunkJsonMap, splunkRemoveUnnecessaryFieldsFilter{})

	if len(structuredJsonMap) == 0 {
		// If the log statement is NOT a GitOps Service controller log statement, then parse the fields as generic splunk output, and return
		return parseSplunkOnly(splunkJsonMap)
	}

	// Parse the log statement as a GitOps Service controller log statment (and parse any splunk fields as well)

	ts, exists := extractAndRemoveStringField("ts", &structuredJsonMap)
	if !exists {
		fmt.Println("Warning: ts not found")
	} else {
		ts = strings.ReplaceAll(ts, "T", " ")
		ts = strings.ReplaceAll(ts, "Z", "z")

		// Pad timestamp to 29
		for {
			if len(ts) >= 29 {
				break
			}

			ts += "0"
		}

	}
	res := fmt.Sprintf("[%s] ", timestampColor(ts))

	if level, exists := extractAndRemoveStringField("level", &structuredJsonMap); !exists {
		fmt.Println("Warning: level not found")
	} else if level == "error" {
		res += fmt.Sprintf(" [%s] ", errorColor("err"))
	} else {
		res += fmt.Sprintf("[%s] ", infoColor(level))
	}

	msg, exists := extractAndRemoveStringField("msg", &structuredJsonMap)
	if !exists {
		fmt.Println("Warning: msg not found")
	}
	res += fmt.Sprintf("%s ", msg)

	// Build the second section: strucutured fields that are recognized as important for our code
	{
		section2 := ""

		// If both namespace and workspace exist, and they are equal, then remove workspace
		namespace, workspace := extractStringField("namespace", structuredJsonMap), extractStringField("workspace", structuredJsonMap)
		if namespace == workspace {
			_, _ = extractAndRemoveStringField("workspace", &structuredJsonMap)
		}

		// Extract important fields out of @other-fields and into the second section
		importantFields := []string{"error", "namespace", "workspace", "name", "component", "job"}

		for _, importantField := range importantFields {

			if str, exists := extractAndRemoveStringField(importantField, &structuredJsonMap); exists {
				section2 += fieldColor(fmt.Sprintf("%s:", importantField)) + wrapStringWithSingleQuotesIfContainsWhitespace(str) + ", "
			}

		}

		section2 = strings.TrimSuffix(section2, ", ")

		if len(section2) > 0 {
			res += separatorColor("|") + " "

			res += section2 + " "
		}
	}

	stackTrace, stackTraceExists := extractAndRemoveStringField("stacktrace", &structuredJsonMap)

	var caller string
	var callerExists bool

	if stackTraceExists {
		caller, callerExists = extractAndRemoveStringField("caller", &structuredJsonMap)
	}

	if len(structuredJsonMap) > 0 {

		res += separatorColor("|") + " "

		res += separatorTextColor("@other-fields: ")

		keys := []string{}
		{

			for mapKey := range structuredJsonMap {

				keys = append(keys, mapKey)

			}

			keys = sortKeysWithFavoredAndDisfavoredFields(keys, []string{"controllerKind", "caller"}, []string{"object", "applicationSpecField", "Application", "logLineNumber"})

		}

		for _, key := range keys {

			val := structuredJsonMap[key]

			if valMap, isMap := (val).(map[string]any); isMap {

				res += fmt.Sprintf("%s{%s}, ",
					fieldColor(wrapStringWithSingleQuotesIfContainsWhitespace(key+":")), recursiveToStringOnJsonMap(valMap))

			} else {

				valStr := fmt.Sprintf("%v", val)

				res += fmt.Sprintf("%s%s, ",
					fieldColor(wrapStringWithSingleQuotesIfContainsWhitespace(key+":")),
					wrapStringWithSingleQuotesIfContainsWhitespace(valStr))

			}

		}

		res = strings.TrimSuffix(res, ", ") + " "

	}

	if len(splunkJsonMap) > 0 {

		res += separatorColor("|") + " "

		res += splunkSeparatorTextColor("@splunk-fields: ")

		res += generateSplunkFieldsOutput(splunkJsonMap)

	}

	res = strings.TrimSpace(res)

	if stackTraceExists {
		res += "\n"
		if callerExists {
			res += indentString("- " + fieldColor("caller: ") + caller)
		}

		res += indentString("- " + fieldColor("stack-trace:"))

		stackTraceLine := strings.TrimSuffix(indentString(stackTrace), "\n")

		res += stackTraceLine
	}

	return res

}

func generateSplunkFieldsOutput(splunkJsonMap map[string]any) string {

	var res string

	// Extract the RHTAP member cluster name
	var rhtapClusterName string
	{

		if kubernetesVal, exists := splunkJsonMap["kubernetes"]; exists {

			kubernetes := (kubernetesVal).(map[string]any)

			if namespaceLabelsVal, exists := kubernetes["namespace_labels"]; exists {

				namespaceLabels := (namespaceLabelsVal).(map[string]any)
				rhtapCluster := (namespaceLabels["app_kubernetes_io_instance"]).(string)
				if rhtapCluster != "" {
					delete(namespaceLabels, "app_kubernetes_io_instance")
				}
				rhtapClusterName = rhtapCluster

				if len(namespaceLabels) == 0 { // if the child map is now empty, delete it from the parent
					delete(kubernetes, "namespace_labels")
				}

			}

		}

	}

	if rhtapClusterName != "" {
		newKey := "rhtapCluster"
		if rhtapClusterName != "" {
			splunkJsonMap[newKey] = rhtapClusterName
		}
	}

	// Print other fields alphabetically
	keys := []string{}
	{
		for mapKey := range splunkJsonMap {
			keys = append(keys, mapKey)
		}

		keys = sortKeysWithFavoredAndDisfavoredFields(keys, []string{"rhtapCluster"}, []string{"object"})

	}

	for _, key := range keys {

		val := splunkJsonMap[key]

		// Skip empty maps
		if mapVal, isMap := (val).(map[string]any); isMap && len(mapVal) == 0 {
			continue
		}

		if valMap, isMap := (val).(map[string]any); isMap {

			res += fmt.Sprintf("%s:{%s}, ",
				fieldColor(wrapStringWithSingleQuotesIfContainsWhitespace(key)), recursiveToStringOnJsonMap(valMap))

		} else {

			valStr := fmt.Sprintf("%v", val)

			res += fmt.Sprintf("%s%s, ",
				fieldColor(wrapStringWithSingleQuotesIfContainsWhitespace(key)+":"),
				wrapStringWithSingleQuotesIfContainsWhitespace(valStr))

		}

	}

	res = strings.TrimSuffix(res, ", ")

	return res
}

func parseSplunkOnly(splunkJsonMap map[string]any) string {

	level, exists := extractAndRemoveStringField("level", &splunkJsonMap)
	if !exists {
		fmt.Println("Warning: level not found")
	}

	ts, exists := extractAndRemoveStringField("@timestamp", &splunkJsonMap)
	if !exists {
		fmt.Println("Warning: @timestamp not found")
	} else {
		ts = strings.ReplaceAll(ts, "T", " ")
		ts = strings.ReplaceAll(ts, "Z", "")

		// Pad timestamp to 29, the default ts size for splunk logs
		for {
			if len(ts) >= 29 {
				break
			}

			ts += "0"
		}
	}

	msg, exists := extractAndRemoveStringField("message", &splunkJsonMap)
	if !exists {
		fmt.Println("Warning: message not found")
	}

	res := fmt.Sprintf("[%s] ", timestampColor(ts))

	if level == "error" {
		res += fmt.Sprintf(" [%s] ", errorColor("err"))
	} else {
		res += fmt.Sprintf("[%s] ", infoColor(level))
	}

	res += fmt.Sprintf("%s ", msg)

	res += separatorColor("|") + " "

	res += separatorTextColor("@splunk-fields: ")

	res += generateSplunkFieldsOutput(splunkJsonMap)

	return res
}

// sortKeysWithFavoredAndDisfavoredFields will:
// - sort the list of map keys in order
// - if a map key is in the favored list, it will move to the front of the list after sort (in order of the favored list)
// - if a map key is in the disfavored list, it will move to the back of the list after sort (in order of the disfavored list)
func sortKeysWithFavoredAndDisfavoredFields(keys []string, favoredFields []string, unfavoredFields []string) []string {

	res := []string{}

	matchingFavoredFields := map[string]any{}
	matchingUnfavoredFields := map[string]any{}

outer:
	for _, key := range keys {

		for _, favoredField := range favoredFields {
			if favoredField == key {
				matchingFavoredFields[key] = key
				continue outer
			}
		}

		for _, unfavoredField := range unfavoredFields {
			if unfavoredField == key {
				matchingUnfavoredFields[key] = key
				continue outer
			}
		}

		// All other fields, add them to the result
		res = append(res, key)

	}

	// Prepend the favored fields to the beginning of the key list, in order
	/// (we do it in reverse order, due to the nature of prepend to beginning of list)
	for i := len(favoredFields) - 1; i >= 0; i-- {

		favoredField := favoredFields[i]

		if _, exists := matchingFavoredFields[favoredField]; exists {
			res = append([]string{favoredField}, res...)
		}

	}
	// Append the unfavored fields, in order
	for _, unfavoredField := range unfavoredFields {

		if _, exists := matchingUnfavoredFields[unfavoredField]; exists {
			res = append(res, unfavoredField)
		}
	}

	return res

}

// read 'fieldName' from the map as a generic object, remove it from the map, then return the object
func extractAndRemoveField(fieldName string, mapVar *map[string]any) (any, bool) {

	value, exists := (*mapVar)[fieldName]

	if !exists {
		return nil, false
	}

	delete((*mapVar), fieldName)

	return value, exists

}

// read 'fieldName' from the map as a string, remove it from the map, then return the string
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

// read a field as a string, or "" if not found or not a string
func extractStringField(fieldName string, mapVar map[string]any) string {

	valueObj, exists := mapVar[fieldName]

	if !exists {
		return ""
	}

	strVal, ok := (valueObj).(string)
	if !ok {
		return ""
	}

	return strVal

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

func wrapStringWithSingleQuotesIfContainsWhitespace(input string) string {

	// Remove EOL
	input = strings.ReplaceAll(input, "\r", "\\r")
	input = strings.ReplaceAll(input, "\n", "\\n")

	if len(input) == 0 {
		return "''"
	}

	if !strings.Contains(input, " ") {
		return input
	}

	return "'" + input + "'"

}

// Recursively convert a JSON map to a string, but unlike fmt.Sprintf("%v",(...)), we use our own output functions for key/value output
func recursiveToStringOnJsonMap(obj map[string]any) string {

	var res string

	for k, v := range obj {

		valMap, isMap := v.(map[string]any)
		if isMap {

			if len(valMap) == 0 {
				continue
			}

			res += fmt.Sprintf("%s:{%s}, ",
				wrapStringWithSingleQuotesIfContainsWhitespace(k), recursiveToStringOnJsonMap(valMap))

		} else {

			res += fmt.Sprintf("%s:%s, ",
				wrapStringWithSingleQuotesIfContainsWhitespace(k),
				wrapStringWithSingleQuotesIfContainsWhitespace(fmt.Sprintf("%v", v)))
		}
	}

	res = strings.TrimSuffix(res, ", ")

	return res
}

type ByTS []string

func (a ByTS) Len() int      { return len(a) }
func (a ByTS) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a ByTS) Less(i, j int) bool {
	return extractTimestampFromLine(a[i]) < extractTimestampFromLine(a[j])
}

func extractTimestampFromLine(line string) string {
	fullJsonMap := map[string]any{}

	var res string

	if err := json.Unmarshal(([]byte)(line), &fullJsonMap); err != nil {
		res = ""
	} else {

		structuredMapVal, exists := fullJsonMap["structured"]
		if exists {

			structuredMap := (structuredMapVal).(map[string]any)

			res = extractStringField("ts", structuredMap)

		} else {

			res = extractStringField("@timestamp", fullJsonMap)
		}

	}

	if res == "" {
		fmt.Println("Unable to extract timestamp from line: " + line)
	}

	return res

}
