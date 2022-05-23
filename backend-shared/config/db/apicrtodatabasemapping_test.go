package db_test

import (
	"context"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	db "github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
)

var _ = Describe("Apicrtodatabasemapping Tests", func() {
	Context("Tests all the DB functions for Apicrtodatabasemapping", func() {
		It("Should execute all Apicrtodatabasemapping Functions", func() {
			err := db.SetupForTestingDBGinkgo()
			Expect(err).To(BeNil())

			item := db.APICRToDatabaseMapping{
				APIResourceType:      db.APICRToDatabaseMapping_ResourceType_GitOpsDeploymentSyncRun,
				APIResourceUID:       "test-k8s-uid",
				APIResourceName:      "test-k8s-name",
				APIResourceNamespace: "test-k8s-namespace",
				NamespaceUID:         "test-namespace-uid",
				DBRelationType:       db.APICRToDatabaseMapping_DBRelationType_SyncOperation,
				DBRelationKey:        "test-key",
			}
			ctx := context.Background()
			dbq, err := db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).To(BeNil())
			defer dbq.CloseDatabase()
			err = dbq.CreateAPICRToDatabaseMapping(ctx, &item)
			Expect(err).To(BeNil())

			fetchRow := db.APICRToDatabaseMapping{
				APIResourceType: item.APIResourceType,
				APIResourceUID:  item.APIResourceUID,
				DBRelationKey:   item.DBRelationKey,
				DBRelationType:  item.DBRelationType,
			}

			err = dbq.GetDatabaseMappingForAPICR(ctx, &fetchRow)
			Expect(err).To(BeNil())
			Expect(fetchRow).Should(Equal(item))

			var items []db.APICRToDatabaseMapping

			err = dbq.ListAPICRToDatabaseMappingByAPINamespaceAndName(ctx, item.APIResourceType, item.APIResourceName, item.APIResourceNamespace, item.NamespaceUID, item.DBRelationType, &items)
			Expect(err).To(BeNil())
			Expect(items[0]).Should(Equal(item))

			rowsAffected, err := dbq.DeleteAPICRToDatabaseMapping(ctx, &fetchRow)
			Expect(err).To(BeNil())
			Expect(rowsAffected).To(Equal((1)))
			fetchRow = db.APICRToDatabaseMapping{
				APIResourceType: item.APIResourceType,
				APIResourceUID:  item.APIResourceUID,
				DBRelationKey:   item.DBRelationKey,
				DBRelationType:  item.DBRelationType,
			}
			err = dbq.GetDatabaseMappingForAPICR(ctx, &fetchRow)
			Expect(db.IsResultNotFoundError(err)).To(Equal(true))

			// Set the invalid value
			item.APIResourceName = strings.Repeat("abc", 100)
			err = dbq.CreateAPICRToDatabaseMapping(ctx, &item)
			Expect(db.IsMaxLengthError(err)).To(Equal(true))

		})
	})
})
