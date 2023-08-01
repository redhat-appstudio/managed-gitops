package db_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	db "github.com/redhat-appstudio/managed-gitops/backend-shared/db"
)

var _ = Describe("Injection Test", func() {
	It("Should test GitopsEngineInstanceWrongInput", func() {
		err := db.SetupForTestingDBGinkgo()
		Expect(err).ToNot(HaveOccurred())
		dbq, err := db.NewUnsafePostgresDBQueries(true, true)
		Expect(err).ToNot(HaveOccurred())
		defer dbq.CloseDatabase()

		ctx := context.Background()

		var clusterUser = &db.ClusterUser{
			Clusteruser_id: "test-user-wrong-application",
			User_name:      "test-user-wrong-application",
		}
		err = dbq.CreateClusterUser(ctx, clusterUser)
		Expect(err).ToNot(HaveOccurred())

		clusterCredentials := db.ClusterCredentials{
			Clustercredentials_cred_id:  "test-cluster-creds-test-5",
			Host:                        "host",
			Kube_config:                 "kube-config",
			Kube_config_context:         "kube-config-context",
			Serviceaccount_bearer_token: "serviceaccount_bearer_token",
			Serviceaccount_ns:           "Serviceaccount_ns",
		}

		managedEnvironment := db.ManagedEnvironment{
			Managedenvironment_id: "test-managed-env-5",
			Clustercredentials_id: clusterCredentials.Clustercredentials_cred_id,
			Name:                  "my env",
		}

		gitopsEngineCluster := db.GitopsEngineCluster{
			Gitopsenginecluster_id: "test-fake-cluster-5",
			Clustercredentials_id:  clusterCredentials.Clustercredentials_cred_id,
		}

		gitopsEngineInstance := db.GitopsEngineInstance{
			Gitopsengineinstance_id: "test-fake-engine-instance-id",
			Namespace_name:          "test'fake'namespace",
			Namespace_uid:           "test-fake-namespace-5",
			EngineCluster_id:        gitopsEngineCluster.Gitopsenginecluster_id,
		}

		clusterAccess := db.ClusterAccess{
			Clusteraccess_user_id:                   clusterUser.Clusteruser_id,
			Clusteraccess_managed_environment_id:    managedEnvironment.Managedenvironment_id,
			Clusteraccess_gitops_engine_instance_id: gitopsEngineInstance.Gitopsengineinstance_id,
		}

		err = dbq.CreateClusterCredentials(ctx, &clusterCredentials)
		Expect(err).ToNot(HaveOccurred())

		err = dbq.CreateManagedEnvironment(ctx, &managedEnvironment)
		Expect(err).ToNot(HaveOccurred())

		err = dbq.CreateGitopsEngineCluster(ctx, &gitopsEngineCluster)
		Expect(err).ToNot(HaveOccurred())

		err = dbq.CreateGitopsEngineInstance(ctx, &gitopsEngineInstance)
		Expect(err).ToNot(HaveOccurred())

		err = dbq.CreateClusterAccess(ctx, &clusterAccess)
		Expect(err).ToNot(HaveOccurred())

		application := &db.Application{
			Application_id:          "test-my-application-5",
			Name:                    "test'application",
			Spec_field:              "{}",
			Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
			Managed_environment_id:  managedEnvironment.Managedenvironment_id,
		}

		err = dbq.CheckedCreateApplication(ctx, application, clusterAccess.Clusteraccess_user_id)
		Expect(err).ToNot(HaveOccurred())

		retrievedApplication := db.Application{Application_id: application.Application_id}

		err = dbq.GetApplicationById(ctx, &retrievedApplication)
		Expect(err).ToNot(HaveOccurred())
		Expect(application.Name).To(Equal(retrievedApplication.Name))
	})

	It("Should test TestClusterCredentialWrongInput", func() {
		err := db.SetupForTestingDBGinkgo()
		Expect(err).ToNot(HaveOccurred())
		dbq, err := db.NewUnsafePostgresDBQueries(true, true)
		Expect(err).ToNot(HaveOccurred())
		defer dbq.CloseDatabase()

		ctx := context.Background()

		clusterCredentials := db.ClusterCredentials{
			Clustercredentials_cred_id: "test-cluster-creds-input",
			Host:                       "host'sInput'",
		}

		err = dbq.CreateClusterCredentials(ctx, &clusterCredentials)
		Expect(err).ToNot(HaveOccurred())
		retrievedClusterCredentials := &db.ClusterCredentials{
			Clustercredentials_cred_id: clusterCredentials.Clustercredentials_cred_id,
		}
		err = dbq.GetClusterCredentialsById(ctx, retrievedClusterCredentials)
		Expect(err).ToNot(HaveOccurred())
		Expect(clusterCredentials.Host).To(Equal(retrievedClusterCredentials.Host))
		Expect(err).ToNot(HaveOccurred())
	})

	It("Should test TestManagedEnviromentWrongInput", func() {
		err := db.SetupForTestingDBGinkgo()
		Expect(err).ToNot(HaveOccurred())
		dbq, err := db.NewUnsafePostgresDBQueries(true, true)
		Expect(err).ToNot(HaveOccurred())
		defer dbq.CloseDatabase()

		ctx := context.Background()

		clusterCredentials := db.ClusterCredentials{
			Clustercredentials_cred_id:  "test-cluster-creds-test-1",
			Host:                        "host",
			Kube_config:                 "kube-config",
			Kube_config_context:         "kube-config-context",
			Serviceaccount_bearer_token: "serviceaccount_bearer_token",
			Serviceaccount_ns:           "Serviceaccount_ns",
		}

		err = dbq.CreateClusterCredentials(ctx, &clusterCredentials)
		Expect(err).ToNot(HaveOccurred())
		{
			managedEnvironment := db.ManagedEnvironment{
				Managedenvironment_id: "test-managed-env-1",
				Clustercredentials_id: clusterCredentials.Clustercredentials_cred_id,
				Name:                  "test'env",
			}
			err = dbq.CreateManagedEnvironment(ctx, &managedEnvironment)
			Expect(err).ToNot(HaveOccurred())
			retrieveManagedEnv := &db.ManagedEnvironment{
				Managedenvironment_id: "test-managed-env-1",
			}
			err = dbq.GetManagedEnvironmentById(ctx, retrieveManagedEnv)
			Expect(err).ToNot(HaveOccurred())
			Expect(managedEnvironment.Name).To(Equal(retrieveManagedEnv.Name))
		}
		Expect(err).ToNot(HaveOccurred())
	})

	It("Should test TestClusterUserWrongInput", func() {
		err := db.SetupForTestingDBGinkgo()
		Expect(err).ToNot(HaveOccurred())
		dbq, err := db.NewUnsafePostgresDBQueries(true, true)
		Expect(err).ToNot(HaveOccurred())
		defer dbq.CloseDatabase()

		ctx := context.Background()

		user := &db.ClusterUser{
			Clusteruser_id: "test-user-id",
			User_name:      "samyak'scluster",
		}
		err = dbq.CreateClusterUser(ctx, user)
		Expect(err).ToNot(HaveOccurred())

		retrieveUser := &db.ClusterUser{
			User_name: "samyak'scluster",
		}
		err = dbq.GetClusterUserByUsername(ctx, retrieveUser)
		Expect(err).ToNot(HaveOccurred())
	})

	It("Should test TestApplicationWrongInput", func() {
		err := db.SetupForTestingDBGinkgo()
		Expect(err).ToNot(HaveOccurred())
		dbq, err := db.NewUnsafePostgresDBQueries(true, true)
		Expect(err).ToNot(HaveOccurred())
		defer dbq.CloseDatabase()

		ctx := context.Background()

		var clusterUser = &db.ClusterUser{
			Clusteruser_id: "test-user-wrong-application",
			User_name:      "test-user-wrong-application",
		}
		err = dbq.CreateClusterUser(ctx, clusterUser)
		Expect(err).ToNot(HaveOccurred())

		clusterCredentials := db.ClusterCredentials{
			Clustercredentials_cred_id:  "test-cluster-creds-test-5",
			Host:                        "host",
			Kube_config:                 "kube-config",
			Kube_config_context:         "kube-config-context",
			Serviceaccount_bearer_token: "serviceaccount_bearer_token",
			Serviceaccount_ns:           "Serviceaccount_ns",
		}

		managedEnvironment := db.ManagedEnvironment{
			Managedenvironment_id: "test-managed-env-5",
			Clustercredentials_id: clusterCredentials.Clustercredentials_cred_id,
			Name:                  "my env",
		}

		gitopsEngineCluster := db.GitopsEngineCluster{
			Gitopsenginecluster_id: "test-fake-cluster-5",
			Clustercredentials_id:  clusterCredentials.Clustercredentials_cred_id,
		}

		gitopsEngineInstance := db.GitopsEngineInstance{
			Gitopsengineinstance_id: "test-fake-engine-instance-id",
			Namespace_name:          "test-fake-namespace",
			Namespace_uid:           "test-fake-namespace-5",
			EngineCluster_id:        gitopsEngineCluster.Gitopsenginecluster_id,
		}

		clusterAccess := db.ClusterAccess{
			Clusteraccess_user_id:                   clusterUser.Clusteruser_id,
			Clusteraccess_managed_environment_id:    managedEnvironment.Managedenvironment_id,
			Clusteraccess_gitops_engine_instance_id: gitopsEngineInstance.Gitopsengineinstance_id,
		}

		err = dbq.CreateClusterCredentials(ctx, &clusterCredentials)
		Expect(err).ToNot(HaveOccurred())

		err = dbq.CreateManagedEnvironment(ctx, &managedEnvironment)
		Expect(err).ToNot(HaveOccurred())

		err = dbq.CreateGitopsEngineCluster(ctx, &gitopsEngineCluster)
		Expect(err).ToNot(HaveOccurred())

		err = dbq.CreateGitopsEngineInstance(ctx, &gitopsEngineInstance)
		Expect(err).ToNot(HaveOccurred())

		err = dbq.CreateClusterAccess(ctx, &clusterAccess)
		Expect(err).ToNot(HaveOccurred())

		application := &db.Application{
			Application_id:          "test-my-application-5",
			Name:                    "test'application",
			Spec_field:              "{}",
			Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
			Managed_environment_id:  managedEnvironment.Managedenvironment_id,
		}

		err = dbq.CheckedCreateApplication(ctx, application, clusterAccess.Clusteraccess_user_id)
		Expect(err).ToNot(HaveOccurred())

		retrievedApplication := db.Application{Application_id: application.Application_id}

		err = dbq.GetApplicationById(ctx, &retrievedApplication)
		Expect(err).ToNot(HaveOccurred())
		Expect(application.Name).To(Equal(retrievedApplication.Name))
	})

})
