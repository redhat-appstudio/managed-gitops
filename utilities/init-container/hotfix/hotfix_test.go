package hotfix

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/db"
)

var _ = Describe("Test for ensuring that hotfix can perform the expected update", func() {

	Context("Update resourceUID field of KubernetesAPIToDBMapping", func() {

		var targetKDB db.KubernetesToDBResourceMapping
		var dbq db.AllDatabaseQueries

		const (
			oldK8sResourceUID = "9dc321a8-aab6-4482-ac71-3c16be1e7c47"
			newK8sResourceUID = "6c91e02f-b6d6-40cf-aa57-7478694f660b"
		)

		BeforeEach(func() {

			targetKDB = db.KubernetesToDBResourceMapping{
				KubernetesResourceType: "Namespace",
				KubernetesResourceUID:  oldK8sResourceUID,
				DBRelationType:         "GitopsEngineInstance",
				DBRelationKey:          "701632d3-dd8b-4173-88c3-84306579a156",
			}

			err := db.SetupForTestingDBGinkgo()
			Expect(err).To(BeNil())

			dbq, err = db.NewUnsafePostgresDBQueries(false, true)
			Expect(err).To(BeNil())

			// Clean up previous test runs
			var results []db.KubernetesToDBResourceMapping
			err = dbq.UnsafeListAllKubernetesResourceToDBResourceMapping(context.Background(), &results)
			Expect(err).To(BeNil())
			for idx := range results {
				result := results[idx]

				if result.DBRelationKey == targetKDB.DBRelationKey {
					results, err := dbq.DeleteKubernetesResourceToDBResourceMapping(context.Background(), &result)
					Expect(err).To(BeNil())
					Expect(results).To(Equal(1))
				}
			}
		})

		It("verifies HotfixK8sResourceUIDOfKubernetesResourceToDBResourceMapping updates Kubernetes UID field, on match", func() {

			By("Creating the target value, and calling hotfix on it")
			err := dbq.CreateKubernetesResourceToDBResourceMapping(context.Background(), &targetKDB)
			Expect(err).To(BeNil())

			err = HotfixK8sResourceUIDOfKubernetesResourceToDBResourceMapping(context.Background(), targetKDB, oldK8sResourceUID, newK8sResourceUID)
			Expect(err).To(BeNil())

			By("verifying the K8sToDBResourceMapping was updated")
			newTargetKDB := db.KubernetesToDBResourceMapping{
				KubernetesResourceType: targetKDB.KubernetesResourceType,
				DBRelationType:         targetKDB.DBRelationType,
				DBRelationKey:          targetKDB.DBRelationKey,
			}

			err = dbq.GetKubernetesResourceMappingForDatabaseResource(context.Background(), &newTargetKDB)
			Expect(err).To(BeNil())

			Expect(newTargetKDB).To(Equal(db.KubernetesToDBResourceMapping{
				KubernetesResourceType: targetKDB.KubernetesResourceType,
				KubernetesResourceUID:  newK8sResourceUID,
				DBRelationType:         targetKDB.DBRelationType,
				DBRelationKey:          targetKDB.DBRelationKey,
				SeqID:                  newTargetKDB.SeqID,
			}), "the KubernetesResourceUID field should have been updated to the new value")

		})

		It("verifies HotfixK8sResourceUIDOfKubernetesResourceToDBResourceMapping does not update unmatched database entries", func() {

			By("creating an entry that doesnt match the parameters, and therefore should not be modified")
			shouldNotBeTouched := db.KubernetesToDBResourceMapping{
				KubernetesResourceType: targetKDB.KubernetesResourceType,
				KubernetesResourceUID:  "test-uid",
				DBRelationType:         targetKDB.DBRelationType,
				DBRelationKey:          targetKDB.DBRelationKey,
			}

			err := dbq.CreateKubernetesResourceToDBResourceMapping(context.Background(), &shouldNotBeTouched)
			Expect(err).To(BeNil())

			By("by calling hotfix on a different value from the 'shouldNotBeTouched' value")
			err = HotfixK8sResourceUIDOfKubernetesResourceToDBResourceMapping(context.Background(), targetKDB, oldK8sResourceUID, newK8sResourceUID)
			Expect(err).To(BeNil())

			By("retrieving the original value, and ensuring it has not been modified")
			getKDB := shouldNotBeTouched
			getKDB.KubernetesResourceUID = ""

			err = dbq.GetKubernetesResourceMappingForDatabaseResource(context.Background(), &getKDB)
			Expect(err).To(BeNil())

			Expect(getKDB).To(Equal(db.KubernetesToDBResourceMapping{
				KubernetesResourceType: shouldNotBeTouched.KubernetesResourceType,
				KubernetesResourceUID:  shouldNotBeTouched.KubernetesResourceUID,
				DBRelationType:         shouldNotBeTouched.DBRelationType,
				DBRelationKey:          shouldNotBeTouched.DBRelationKey,
				SeqID:                  getKDB.SeqID,
			}), "the resource should not have changed")

		})

	})

})
