package util

import (
	"testing"

	"github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
	"github.com/stretchr/testify/assert"
)

func TestUncheckedGetOrCreateGitopsEngineInstanceByInstanceNamespaceUID(t *testing.T) {

	_, err := db.NewUnsafePostgresDBQueries(true, true)
	if !assert.Nil(t, err) {
		return
	}

	// TODO: DEBT - FIX Me!

	// clusterCredentials := db.ClusterCredentials{
	// 	Clustercredentials_cred_id:  "test-creds",
	// 	Host:                        "",
	// 	Kube_config:                 "",
	// 	Kube_config_context:         "",
	// 	Serviceaccount_bearer_token: "",
	// 	Serviceaccount_ns:           "",
	// }

	// gitopsEngineNamespace := v1.Namespace{
	// 	ObjectMeta: metav1.ObjectMeta{
	// 		Name: "fake-namespace",
	// 		UID:  "fake-uid",
	// 	},
	// }

	// kubesystemNamespaceUID := "fake-uid"

	// engineInstance, engineCluster, err := UncheckedGetOrCreateGitopsEngineInstanceByInstanceNamespaceUID(context.Background(), gitopsEngineNamespace,
	// 	kubesystemNamespaceUID, clusterCredentials, dbq, logr.FromContext(context.Background()))

	// t.Logf("%v", engineInstance)
	// t.Logf("%v", engineCluster)

	// assert.Nil(t, err)
}
