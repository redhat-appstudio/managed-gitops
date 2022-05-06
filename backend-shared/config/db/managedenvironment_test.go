package db_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	db "github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
)

var _ = Describe("Managedenvironment Test", func() {
	It("Should Create, Get and Delete a ManagedEnvironment", func() {
		err := db.SetupForTestingDBGinkgo()
		Expect(err).To(BeNil())

		ctx := context.Background()
		dbq, err := db.NewUnsafePostgresDBQueries(true, true)
		Expect(err).To(BeNil())
		defer dbq.CloseDatabase()

		clusterCredentials := db.ClusterCredentials{
			Clustercredentials_cred_id:  "test-cluster-creds-test-3",
			Host:                        "host",
			Kube_config:                 "kube-config",
			Kube_config_context:         "kube-config-context",
			Serviceaccount_bearer_token: "serviceaccount_bearer_token",
			Serviceaccount_ns:           "Serviceaccount_ns",
		}

		managedEnvironment := db.ManagedEnvironment{
			Managedenvironment_id: "test-managed-env-3",
			Clustercredentials_id: clusterCredentials.Clustercredentials_cred_id,
			Name:                  "my env101",
		}

		err = dbq.CreateClusterCredentials(ctx, &clusterCredentials)
		Expect(err).To(BeNil())

		err = dbq.CreateManagedEnvironment(ctx, &managedEnvironment)
		Expect(err).To(BeNil())

		getmanagedEnvironment := db.ManagedEnvironment{
			Managedenvironment_id: managedEnvironment.Managedenvironment_id,
			SeqID:                 managedEnvironment.SeqID,
			Name:                  managedEnvironment.Name,
			Clustercredentials_id: managedEnvironment.Clustercredentials_id,
		}
		err = dbq.GetManagedEnvironmentById(ctx, &getmanagedEnvironment)
		Expect(err).To(BeNil())
		Expect(managedEnvironment).Should(Equal(getmanagedEnvironment))

		rowsAffected, err := dbq.DeleteManagedEnvironmentById(ctx, getmanagedEnvironment.Managedenvironment_id)
		Expect(err).To(BeNil())
		Expect(rowsAffected).Should(Equal(1))

		err = dbq.GetManagedEnvironmentById(ctx, &getmanagedEnvironment)
		Expect(true).To(Equal(db.IsResultNotFoundError(err)))

	})

	It("Should List all the ManagedEnvironment entries", func() {
		ctx := context.Background()
		dbq, err := db.NewUnsafePostgresDBQueries(true, true)
		Expect(err).To(BeNil())
		defer dbq.CloseDatabase()

		var clusterUser = &db.ClusterUser{
			Clusteruser_id: "test-user-application",
			User_name:      "test-user-application",
		}

		clusterCredentials := db.ClusterCredentials{
			Clustercredentials_cred_id:  "test-cluster-creds-test-6",
			Host:                        "host",
			Kube_config:                 "kube-config",
			Kube_config_context:         "kube-config-context",
			Serviceaccount_bearer_token: "serviceaccount_bearer_token",
			Serviceaccount_ns:           "Serviceaccount_ns",
		}

		managedEnvironment := db.ManagedEnvironment{
			Managedenvironment_id: "test-managed-env-6",
			Clustercredentials_id: clusterCredentials.Clustercredentials_cred_id,
			Name:                  "my env101",
		}

		gitopsEngineCluster := db.GitopsEngineCluster{
			Gitopsenginecluster_id: "test-fake-cluster-6",
			Clustercredentials_id:  clusterCredentials.Clustercredentials_cred_id,
		}

		gitopsEngineInstance := db.GitopsEngineInstance{
			Gitopsengineinstance_id: "test-fake-engine-instance-id",
			Namespace_name:          "test-fake-namespace",
			Namespace_uid:           "test-fake-namespace-6",
			EngineCluster_id:        gitopsEngineCluster.Gitopsenginecluster_id,
		}

		clusterAccess := db.ClusterAccess{
			Clusteraccess_user_id:                   clusterUser.Clusteruser_id,
			Clusteraccess_managed_environment_id:    managedEnvironment.Managedenvironment_id,
			Clusteraccess_gitops_engine_instance_id: gitopsEngineInstance.Gitopsengineinstance_id,
		}

		var managedEnvironmentget []db.ManagedEnvironment

		err = dbq.CreateClusterUser(ctx, clusterUser)
		Expect(err).To(BeNil())

		err = dbq.CreateClusterCredentials(ctx, &clusterCredentials)
		Expect(err).To(BeNil())

		err = dbq.CreateManagedEnvironment(ctx, &managedEnvironment)
		Expect(err).To(BeNil())

		err = dbq.CreateGitopsEngineCluster(ctx, &gitopsEngineCluster)
		Expect(err).To(BeNil())

		err = dbq.CreateGitopsEngineInstance(ctx, &gitopsEngineInstance)
		Expect(err).To(BeNil())

		err = dbq.CreateClusterAccess(ctx, &clusterAccess)
		Expect(err).To(BeNil())

		err = dbq.ListManagedEnvironmentForClusterCredentialsAndOwnerId(ctx, clusterCredentials.Clustercredentials_cred_id, clusterAccess.Clusteraccess_user_id, &managedEnvironmentget)
		Expect(err).To(BeNil())

		Expect(managedEnvironmentget[0]).Should(Equal(managedEnvironment))
		Expect(len(managedEnvironmentget)).Should(Equal(1))

	})

})
