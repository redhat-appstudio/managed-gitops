package db_test

import (
	"context"
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	db "github.com/redhat-appstudio/managed-gitops/backend-shared/db"
)

var _ = Describe("Kubernetesresourcetodbresourcemapping Test", func() {

	var ctx context.Context
	var dbq db.AllDatabaseQueries
	var kubernetesToDBResourceMapping db.KubernetesToDBResourceMapping

	BeforeEach(func() {
		err := db.SetupForTestingDBGinkgo()
		Expect(err).ToNot(HaveOccurred())

		ctx = context.Background()

		dbq, err = db.NewUnsafePostgresDBQueries(true, true)
		Expect(err).ToNot(HaveOccurred())

		kubernetesToDBResourceMapping = db.KubernetesToDBResourceMapping{
			KubernetesResourceType: "test-resource_2",
			KubernetesResourceUID:  "test-resource_uid",
			DBRelationType:         "test-relation_type",
			DBRelationKey:          "test-relation_key",
		}
		err = dbq.CreateKubernetesResourceToDBResourceMapping(ctx, &kubernetesToDBResourceMapping)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		defer dbq.CloseDatabase()
	})

	It("Should Create, Get, and Delete a KubernetesToDBResourceMapping", func() {

		kubernetesToDBResourceMappingget := db.KubernetesToDBResourceMapping{
			KubernetesResourceType: kubernetesToDBResourceMapping.KubernetesResourceType,
			KubernetesResourceUID:  kubernetesToDBResourceMapping.KubernetesResourceUID,
			DBRelationType:         kubernetesToDBResourceMapping.DBRelationType,
		}

		err := dbq.GetDBResourceMappingForKubernetesResource(ctx, &kubernetesToDBResourceMappingget)
		Expect(err).ToNot(HaveOccurred())
		Expect(kubernetesToDBResourceMappingget).Should(Equal(kubernetesToDBResourceMapping))

		kubernetesToDBResourceMappingget = db.KubernetesToDBResourceMapping{
			KubernetesResourceType: kubernetesToDBResourceMapping.KubernetesResourceType,
			DBRelationType:         kubernetesToDBResourceMapping.DBRelationType,
			DBRelationKey:          "test-relation_key",
		}

		err = dbq.GetKubernetesResourceMappingForDatabaseResource(ctx, &kubernetesToDBResourceMappingget)
		Expect(err).ToNot(HaveOccurred())
		Expect(kubernetesToDBResourceMappingget).Should(Equal(kubernetesToDBResourceMapping))

		rowsAffected, err := dbq.DeleteKubernetesResourceToDBResourceMapping(ctx, &kubernetesToDBResourceMapping)
		Expect(err).ToNot(HaveOccurred())
		Expect(rowsAffected).Should(Equal(1))

		err = dbq.GetDBResourceMappingForKubernetesResource(ctx, &kubernetesToDBResourceMappingget)
		Expect(true).To(Equal(db.IsResultNotFoundError(err)))

		kubernetesToDBResourceMappingNotExist := db.KubernetesToDBResourceMapping{
			KubernetesResourceType: "test-resource_2_not_exist",
			KubernetesResourceUID:  "test-resource_uid_not_exist",
			DBRelationType:         "test-relation_type_not_exist",
			DBRelationKey:          "test-relation_key_not_exist",
		}
		err = dbq.GetDBResourceMappingForKubernetesResource(ctx, &kubernetesToDBResourceMappingNotExist)
		Expect(true).To(Equal(db.IsResultNotFoundError(err)))

		kubernetesToDBResourceMapping.DBRelationType = strings.Repeat("abc", 100)
		err = dbq.CreateKubernetesResourceToDBResourceMapping(ctx, &kubernetesToDBResourceMapping)
		Expect(true).To(Equal(db.IsMaxLengthError(err)))

	})

	It("Should not update a KubernetesResourceUID field if it doesn't exist", func() {

		By("attempting to update a KubernetesToDBResourceMapping that doesnt exist")
		kubernetesToDBResourceMapping := db.KubernetesToDBResourceMapping{
			KubernetesResourceType: "Namespace",
			KubernetesResourceUID:  "test-resource_uid",
			DBRelationType:         db.K8sToDBMapping_GitopsEngineInstance,
			DBRelationKey:          "test-relation_type",
		}
		err := dbq.UpdateKubernetesResourceUIDForKubernetesToDBResourceMapping(ctx, &kubernetesToDBResourceMapping)
		Expect(err).To(HaveOccurred())

	})

	It("Should update the KubernetesResourceUID field of a matching KubernetesToDBResourceMapping, and not update any other values", func() {

		By("creating two similar KubernetesToDBResourceMapping values")
		mapping := []db.KubernetesToDBResourceMapping{}
		for len(mapping) < 2 {

			index := len(mapping)

			kubernetesToDBResourceMapping := db.KubernetesToDBResourceMapping{
				KubernetesResourceType: "Namespace",
				KubernetesResourceUID:  fmt.Sprintf("test-resource_uid_%d", index),
				DBRelationType:         db.K8sToDBMapping_GitopsEngineInstance,
				DBRelationKey:          fmt.Sprintf("test-relation_key_%d", index),
			}
			mapping = append(mapping, kubernetesToDBResourceMapping)

			err := dbq.CreateKubernetesResourceToDBResourceMapping(ctx, &kubernetesToDBResourceMapping)
			Expect(err).ToNot(HaveOccurred())
		}

		shouldNotChange := mapping[1] // the second entry in the DB should not be updated
		toUpdateBefore := mapping[0]

		By("By updating one of the two values")
		toUpdate := mapping[0]
		toUpdate.KubernetesResourceUID = "new-value"
		err := dbq.UpdateKubernetesResourceUIDForKubernetesToDBResourceMapping(ctx, &toUpdate)
		Expect(err).ToNot(HaveOccurred())

		By("retrieving the value after update, and verifying it has been updated")
		err = dbq.GetKubernetesResourceMappingForDatabaseResource(ctx, &toUpdate)
		Expect(err).ToNot(HaveOccurred())

		Expect(toUpdate).To(Equal(db.KubernetesToDBResourceMapping{
			KubernetesResourceType: toUpdateBefore.KubernetesResourceType,
			KubernetesResourceUID:  "new-value",
			DBRelationType:         toUpdateBefore.DBRelationType,
			DBRelationKey:          toUpdateBefore.DBRelationKey,
			SeqID:                  toUpdate.SeqID,
		}))

		By("retrieving the value of the value that should not have been updated, and ensuring it wasn't updated")
		shouldNotChangeNew := mapping[1]
		err = dbq.GetKubernetesResourceMappingForDatabaseResource(ctx, &shouldNotChangeNew)
		Expect(err).ToNot(HaveOccurred())
		shouldNotChange.SeqID = shouldNotChangeNew.SeqID
		Expect(shouldNotChangeNew).To(Equal(shouldNotChange))

	})

	Context("Test Dispose function for kubernetesToDBResourceMapping", func() {
		It("Should test Dispose function with missing database interface for kubernetesToDBResourceMapping", func() {
			var dbq db.AllDatabaseQueries

			err := kubernetesToDBResourceMapping.Dispose(ctx, dbq)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing database interface in KubernetesToDBResourceMapping dispose"))

		})

		It("Should test Dispose function for kubernetesToDBResourceMapping", func() {
			err := kubernetesToDBResourceMapping.Dispose(context.Background(), dbq)
			Expect(err).ToNot(HaveOccurred())

			err = dbq.GetKubernetesResourceMappingForDatabaseResource(ctx, &kubernetesToDBResourceMapping)
			Expect(err).To(HaveOccurred())

		})
	})
})
