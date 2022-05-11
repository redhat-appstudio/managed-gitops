package util

import (
	"context"
	"testing"

	"github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	"sigs.k8s.io/controller-runtime/pkg/log"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Test utility functions.", func() {
	Context("It should execute and test CreateKubernetesResourceToDBResourceMapping function.", func() {

		It("Should create new managedEnvironment and other resources, if called second time then it should return existing resources.", func() {

			workSpaceUid := uuid.NewUUID()
			ctx := context.Background()

			dbQueries, err := db.NewUnsafePostgresDBQueries(false, false)
			Expect(err).To(BeNil())
			defer dbQueries.CloseDatabase()

			workspace := v1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-user",
					UID:       workSpaceUid,
					Namespace: "test-namespace",
				},
				Spec: v1.NamespaceSpec{},
			}

			// ----------------------------------------------------------------------------
			By("It should create new ManagedEnvironment for given namespace.")
			// ----------------------------------------------------------------------------
			log := log.FromContext(context.Background())

			managedEnvironment, isNew, err := GetOrCreateManagedEnvironmentByNamespaceUID(ctx, workspace, dbQueries, log)
			Expect(err).To(BeNil())
			Expect(isNew).To(BeTrue())

			// ----------------------------------------------------------------------------
			By("Verify that other database entries are also created.")
			// ----------------------------------------------------------------------------

			// Check KubernetesToDBResourceMapping resource
			kubernetesToDBResourceMappingget := db.KubernetesToDBResourceMapping{
				KubernetesResourceType: "Namespace",
				KubernetesResourceUID:  string(workSpaceUid),
				DBRelationType:         "ManagedEnvironment",
			}

			err = dbQueries.GetDBResourceMappingForKubernetesResource(ctx, &kubernetesToDBResourceMappingget)
			Expect(err).To(BeNil())

			// Check ClusterCredentials resource
			clusterCredentials := db.ClusterCredentials{
				Clustercredentials_cred_id: managedEnvironment.Clustercredentials_id,
			}
			err = dbQueries.GetClusterCredentialsById(ctx, &clusterCredentials)
			Expect(err).To(BeNil())

			// ----------------------------------------------------------------------------
			By("If called 2nd time, it should return existing ManagedEnvironment instead of creating new and flag should be False.")
			// ----------------------------------------------------------------------------
			managedEnvironmentSecond, isNewSecond, errSecond := GetOrCreateManagedEnvironmentByNamespaceUID(ctx, workspace, dbQueries, log)
			Expect(errSecond).To(BeNil())
			Expect(isNewSecond).To(BeFalse())
			Expect(managedEnvironmentSecond).To(Equal(managedEnvironment))

			// ----------------------------------------------------------------------------
			By("Delete resources created by test.")
			// ----------------------------------------------------------------------------

			// Delete managedEnv
			rowsAffected, err := dbQueries.DeleteManagedEnvironmentById(ctx, managedEnvironment.Managedenvironment_id)
			Expect(err).To(BeNil())
			Expect(rowsAffected).To(Equal(1))

			// Delete clusterCredentials
			rowsAffected, err = dbQueries.DeleteClusterCredentialsById(ctx, clusterCredentials.Clustercredentials_cred_id)

			Expect(err).To(BeNil())
			Expect(rowsAffected).To(Equal(1))

			rowsAffected, err = dbQueries.DeleteKubernetesResourceToDBResourceMapping(ctx, &kubernetesToDBResourceMappingget)
			Expect(err).To(BeNil())
			Expect(rowsAffected).To(Equal(1))
		})

		It("Should fail as NameSpace details are invalid.", func() {

			ctx := context.Background()

			dbQueries, err := db.NewUnsafePostgresDBQueries(false, false)
			Expect(err).To(BeNil())

			workspace := v1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-user",
					UID:       "",
					Namespace: "test-namespace",
				},
				Spec: v1.NamespaceSpec{},
			}

			log := log.FromContext(context.Background())

			managedEnvironment, isNew, err := GetOrCreateManagedEnvironmentByNamespaceUID(ctx, workspace, dbQueries, log)
			Expect(err).NotTo(BeNil())
			Expect(isNew).To(BeFalse())
			Expect(managedEnvironment).To(BeNil())
		})
	})
})

func TestGetOrCreateGitopsEngineInstanceByInstanceNamespaceUID(t *testing.T) {

	_, err := db.NewUnsafePostgresDBQueries(true, true)
	if !assert.Nil(t, err) {
		return
	}

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

	// engineInstance, engineCluster, err := GetOrCreateGitopsEngineInstanceByInstanceNamespaceUID(context.Background(), gitopsEngineNamespace,
	// 	kubesystemNamespaceUID, dbq, logr.FromContext(context.Background()))

	// t.Logf("%v", engineInstance)
	// t.Logf("%v", engineCluster)

	// assert.Nil(t, err)
}
