package db_test

import (
	"context"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	db "github.com/redhat-appstudio/managed-gitops/backend-shared/db"
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
})
