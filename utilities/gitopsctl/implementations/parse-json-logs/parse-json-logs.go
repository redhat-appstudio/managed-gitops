package parsejsonlogs

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/fatih/color"
)

var (
	timestampColor           = color.New(color.FgHiWhite).Add(color.Bold).SprintFunc()
	infoColor                = color.New(color.FgBlue).Add(color.Bold).SprintFunc()
	errorColor               = color.New(color.FgRed).Add(color.Bold).SprintFunc()
	separatorColor           = color.New(color.FgCyan).Add(color.Bold).SprintFunc()
	fieldColor               = color.New(color.FgGreen).SprintFunc()
	splunkSeparatorTextColor = color.New(color.FgYellow).Add(color.Bold).SprintFunc()
	separatorTextColor       = color.New(color.FgMagenta).Add(color.Bold).SprintFunc()
)

func ParseJsonLogs() {

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Text()

		parseJSONLogLine(line)
	}

	if scanner.Err() != nil {
		fmt.Println(scanner.Err().Error())
		os.Exit(1)
		return
	}
}

func parseJSONLogLine(line string) {

	fullJsonMap := map[string]any{}

	if err := json.Unmarshal(([]byte)(line), &fullJsonMap); err != nil {
		fmt.Println(err)
		return
	}

	var structuredMap map[string]any

	structuredMapVal, exists := fullJsonMap["structured"]
	if exists {

		// A) If the 'structured' field exists, then the log is from splunk

		structuredMap = (structuredMapVal).(map[string]any)

		delete(fullJsonMap, "structured")

		ParseMap(structuredMap, fullJsonMap)
	} else {

		_, hasTimestampField := fullJsonMap["@timestamp"]

		if hasTimestampField {

			// C) If the 'structured' field doesn't exist and @timestamp DOES exist, then the log is direct splunk output
			ParseMap(make(map[string]any), fullJsonMap)
		} else {
			// C) If the 'structured' field doesn't exist and @timestamp doesn't exist, then the log is direct controller output (e.g. not from splunk)
			ParseMap(fullJsonMap, make(map[string]any))
		}

	}

}

func ParseMap(structuredJsonMap map[string]any, splunkJsonMap map[string]any) {

	// The splunk logs contain a lot of useless field, so filter them out
	splunkJsonMap = filterByMapKey(splunkJsonMap, splunkRemoveUnnecessaryFieldsFilter{})

	if len(structuredJsonMap) == 0 {
		// If the log statement is NOT a GitOps Service controller log statement, then parse the fields as generic splunk output, and return
		parseSplunkOnly(splunkJsonMap)
		return
	}

	// Parse the log statement as a GitOps Service controller log statment (and parse any splunk fields as well)

	ts, exists := extractAndRemoveStringField("ts", &structuredJsonMap)
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

			keys = sortKeysWithFavoredAndDisfavoredFields(keys, []string{"controllerKind", "caller"}, []string{"object", "applicationSpecField", "Application"})

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

	fmt.Println()
	fmt.Println(res)

	if stackTraceExists {
		if callerExists {
			fmt.Print(indentString("- " + fieldColor("caller: ") + caller))
		}

		fmt.Print(indentString("- " + fieldColor("stack-trace:")))
		fmt.Print(indentString(stackTrace))
	}

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

		{

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

	}

	res = strings.TrimSuffix(res, ", ")

	return res
}

func parseSplunkOnly(splunkJsonMap map[string]any) {

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

		// Pad timestamp to 29
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

	fmt.Println()
	fmt.Println(res)
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

// func recursiveConvertJsonStringsToMaps(obj map[string]any) map[string]any {

// 	res := map[string]any{}

// 	for k, v := range obj {

// 		if str, isString := v.(string); isString {

// 			jsonMap := map[string]any{}

// 			if err := json.Unmarshal(([]byte)(str), &jsonMap); err == nil {

// 				res[k] = recursiveConvertJsonStringsToMaps(jsonMap)

// 			} else {
// 				res[k] = v
// 			}

// 		} else {

// 			res[k] = v
// 		}

// 	}

// 	return res
// }
