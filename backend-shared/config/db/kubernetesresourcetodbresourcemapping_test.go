package db_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	db "github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
)

var _ = Describe("Kubernetesresourcetodbresourcemapping Test", func() {
	It("Should Create, Get and Delete a KubernetesToDBResourceMapping", func() {
		err := db.SetupForTestingDBGinkgo()
		Expect(err).To(BeNil())

		ctx := context.Background()
		dbq, err := db.NewUnsafePostgresDBQueries(true, true)
		Expect(err).To(BeNil())
		defer dbq.CloseDatabase()

		kubernetesToDBResourceMapping := db.KubernetesToDBResourceMapping{
			KubernetesResourceType: "test-resource_2",
			KubernetesResourceUID:  "test-resource_uid",
			DBRelationType:         "test-relation_type",
			DBRelationKey:          "test-relation_key",
		}
		err = dbq.CreateKubernetesResourceToDBResourceMapping(ctx, &kubernetesToDBResourceMapping)
		Expect(err).To(BeNil())

		kubernetesToDBResourceMappingget := db.KubernetesToDBResourceMapping{
			KubernetesResourceType: kubernetesToDBResourceMapping.KubernetesResourceType,
			KubernetesResourceUID:  kubernetesToDBResourceMapping.KubernetesResourceUID,
			DBRelationType:         kubernetesToDBResourceMapping.DBRelationType,
			DBRelationKey:          kubernetesToDBResourceMapping.DBRelationType,
		}

		err = dbq.GetDBResourceMappingForKubernetesResource(ctx, &kubernetesToDBResourceMappingget)
		Expect(err).To(BeNil())
		Expect(kubernetesToDBResourceMappingget).Should(Equal(kubernetesToDBResourceMapping))

		rowsAffected, err := dbq.DeleteKubernetesResourceToDBResourceMapping(ctx, &kubernetesToDBResourceMapping)
		Expect(err).To(BeNil())
		Expect(rowsAffected).Should(Equal(1))

		err = dbq.GetDBResourceMappingForKubernetesResource(ctx, &kubernetesToDBResourceMappingget)
		Expect(true).To(Equal(db.IsResultNotFoundError(err)))
	})
})
