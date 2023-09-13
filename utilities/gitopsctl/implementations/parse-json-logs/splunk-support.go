package parsejsonlogs

import "strings"

func filterByMapKey(mapEntry map[string]any, filter mapKeyFilter) map[string]any {

	newMap := map[string]any{}

	for k, v := range mapEntry {

		if v == nil {
			continue
		}

		if !filter.excludeKey(k) {

			childMap, ok := v.(map[string]any)

			if !ok {
				newMap[k] = v
				continue
			} else {

				res := filterByMapKey(childMap, filter)

				if len(res) > 0 { // ignore empty maps
					newMap[k] = res
				}

			}
		}
	}

	return newMap
}

type mapKeyFilter interface {
	excludeKey(key string) bool
}

type splunkRemoveUnnecessaryFieldsFilter struct {
}

// excludeKey filters any JSON fields that are known to not be useful to GitOps Service developers, when examining RHTAP logs from Splunk
func (filter splunkRemoveUnnecessaryFieldsFilter) excludeKey(key string) bool {

	excludedPhrases := []string{
		"networks-status", "network-status", "pod-networks", "pod-security",
		"olm_operatorgroup", "openshift.io/scc", "container_image", "seccomp",
		"controller-revision-hash", "file", "container_id", "pod_ip",
		"image_upgraded", "cluster-monitoring", "pod_owner", "hostname",
		"argocd.argoproj.io/sync-options", "pod-template-hash", "cluster_id", "sequence",
		"kubernetes_io_metadata_name", "control-plane",
	}

	for _, excludedPhrase := range excludedPhrases {

		if strings.Contains(key, excludedPhrase) {
			return true
		}

	}

	return false
}
