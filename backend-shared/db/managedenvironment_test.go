package db_test

import (
	"context"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	db "github.com/redhat-appstudio/managed-gitops/backend-shared/db"
)

var _ = Describe("Managedenvironment Test", func() {
	var ctx context.Context
	var clusterCredentials db.ClusterCredentials
	var managedEnvironment db.ManagedEnvironment
	var dbq db.AllDatabaseQueries
	BeforeEach(func() {
		err := db.SetupForTestingDBGinkgo()
		Expect(err).ToNot(HaveOccurred())

		ctx = context.Background()
		dbq, err = db.NewUnsafePostgresDBQueries(true, true)
		Expect(err).ToNot(HaveOccurred())

		clusterCredentials = db.ClusterCredentials{
			Clustercredentials_cred_id:  "test-cluster-creds-test-3",
			Host:                        "host",
			Kube_config:                 "kube-config",
			Kube_config_context:         "kube-config-context",
			Serviceaccount_bearer_token: "serviceaccount_bearer_token",
			Serviceaccount_ns:           "Serviceaccount_ns",
		}

		managedEnvironment = db.ManagedEnvironment{
			Managedenvironment_id: "test-managed-env-3",
			Clustercredentials_id: clusterCredentials.Clustercredentials_cred_id,
			Name:                  "my env101",
		}

		err = dbq.CreateClusterCredentials(ctx, &clusterCredentials)
		Expect(err).ToNot(HaveOccurred())

		err = dbq.CreateManagedEnvironment(ctx, &managedEnvironment)
		Expect(err).ToNot(HaveOccurred())
	})
	AfterEach(func() {
		defer dbq.CloseDatabase()
	})
	It("Should Create, Get and Delete a ManagedEnvironment", func() {

		getmanagedEnvironment := db.ManagedEnvironment{
			Managedenvironment_id: managedEnvironment.Managedenvironment_id,
			SeqID:                 managedEnvironment.SeqID,
			Name:                  managedEnvironment.Name,
			Clustercredentials_id: managedEnvironment.Clustercredentials_id,
		}
		err := dbq.GetManagedEnvironmentById(ctx, &getmanagedEnvironment)
		Expect(err).ToNot(HaveOccurred())
		Expect(managedEnvironment.Created_on.After(time.Now().Add(time.Minute*-5))).To(BeTrue(), "Created on should be within the last 5 minutes")
		managedEnvironment.Created_on = getmanagedEnvironment.Created_on
		Expect(managedEnvironment).Should(Equal(getmanagedEnvironment))

		rowsAffected, err := dbq.DeleteManagedEnvironmentById(ctx, getmanagedEnvironment.Managedenvironment_id)
		Expect(err).ToNot(HaveOccurred())
		Expect(rowsAffected).Should(Equal(1))

		err = dbq.GetManagedEnvironmentById(ctx, &getmanagedEnvironment)
		Expect(true).To(Equal(db.IsResultNotFoundError(err)))

		managedEnvironmentNotExist := db.ManagedEnvironment{
			Managedenvironment_id: "test-managed-env-4-not-exist",
			Clustercredentials_id: clusterCredentials.Clustercredentials_cred_id,
			Name:                  "my env101-not-exist",
		}
		err = dbq.GetManagedEnvironmentById(ctx, &managedEnvironmentNotExist)
		Expect(true).To(Equal(db.IsResultNotFoundError(err)))

		managedEnvironment.Clustercredentials_id = strings.Repeat("abc", 100)
		err = dbq.CreateManagedEnvironment(ctx, &managedEnvironment)
		Expect(true).To(Equal(db.IsMaxLengthError(err)))

	})

	It("Should List all the ManagedEnvironment entries", func() {

		_, _, _, gitopsEngineInstance, _, err := db.CreateSampleData(dbq)
		Expect(err).ToNot(HaveOccurred())

		var testClusterUser = &db.ClusterUser{
			Clusteruser_id: "test-user-1",
			User_name:      "test-user-1",
		}
		managedEnvironmentput := db.ManagedEnvironment{
			Managedenvironment_id: "test-managed-env",
			Clustercredentials_id: clusterCredentials.Clustercredentials_cred_id,
			Name:                  "my env",
		}
		clusterAccessput := db.ClusterAccess{
			Clusteraccess_user_id:                   testClusterUser.Clusteruser_id,
			Clusteraccess_managed_environment_id:    managedEnvironmentput.Managedenvironment_id,
			Clusteraccess_gitops_engine_instance_id: gitopsEngineInstance.Gitopsengineinstance_id,
		}
		err = dbq.CreateManagedEnvironment(ctx, &managedEnvironmentput)
		Expect(err).ToNot(HaveOccurred())
		err = dbq.CreateClusterUser(ctx, testClusterUser)
		Expect(err).ToNot(HaveOccurred())
		err = dbq.CreateClusterAccess(ctx, &clusterAccessput)
		Expect(err).ToNot(HaveOccurred())

		var managedEnvironmentget []db.ManagedEnvironment

		err = dbq.ListManagedEnvironmentForClusterCredentialsAndOwnerId(ctx, clusterCredentials.Clustercredentials_cred_id, clusterAccessput.Clusteraccess_user_id, &managedEnvironmentget)
		Expect(err).ToNot(HaveOccurred())

		Expect(managedEnvironmentget[0].Created_on.After(time.Now().Add(time.Minute*-5))).To(BeTrue(), "Created on should be within the last 5 minutes")
		managedEnvironmentget[0].Created_on = managedEnvironmentput.Created_on
		Expect(managedEnvironmentget[0]).Should(Equal(managedEnvironmentput))
		Expect(managedEnvironmentget).Should(HaveLen(1))

	})

	Context("Test Dispose function for managedEnvironment", func() {
		It("Should test Dispose function with missing database interface", func() {
			var dbq db.AllDatabaseQueries

			err := managedEnvironment.Dispose(ctx, dbq)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing database interface in ManagedEnvironment dispose"))

		})

		It("Should test Dispose function for managedEnvironment", func() {

			err := managedEnvironment.Dispose(context.Background(), dbq)
			Expect(err).ToNot(HaveOccurred())

			err = dbq.GetManagedEnvironmentById(ctx, &managedEnvironment)
			Expect(err).To(HaveOccurred())

		})
	})

})
