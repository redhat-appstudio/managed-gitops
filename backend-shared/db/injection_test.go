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
		Expect(err).To(BeNil())
		dbq, err := db.NewUnsafePostgresDBQueries(true, true)
		Expect(err).To(BeNil())
		defer dbq.CloseDatabase()

		ctx := context.Background()

		var clusterUser = &db.ClusterUser{
			ClusterUserID: "test-user-wrong-application",
			UserName:      "test-user-wrong-application",
		}
		err = dbq.CreateClusterUser(ctx, clusterUser)
		Expect(err).To(BeNil())

		clusterCredentials := db.ClusterCredentials{
			ClustercredentialsCredID:  "test-cluster-creds-test-5",
			Host:                      "host",
			KubeConfig:                "kube-config",
			KubeConfig_context:        "kube-config-context",
			ServiceAccountBearerToken: "serviceaccount_bearer_token",
			ServiceAccountNs:          "ServiceAccountNs",
		}

		managedEnvironment := db.ManagedEnvironment{
			Managedenvironment_id: "test-managed-env-5",
			ClusterCredentialsID:  clusterCredentials.ClustercredentialsCredID,
			Name:                  "my env",
		}

		gitopsEngineCluster := db.GitopsEngineCluster{
			PrimaryKeyID:         "test-fake-cluster-5",
			ClusterCredentialsID: clusterCredentials.ClustercredentialsCredID,
		}

		gitopsEngineInstance := db.GitopsEngineInstance{
			Gitopsengineinstance_id: "test-fake-engine-instance-id",
			NamespaceName:           "test'fake'namespace",
			NamespaceUID:            "test-fake-namespace-5",
			EngineCluster_id:        gitopsEngineCluster.PrimaryKeyID,
		}

		clusterAccess := db.ClusterAccess{
			ClusterAccessUserID:                 clusterUser.ClusterUserID,
			ClusterAccessManagedEnvironmentID:   managedEnvironment.Managedenvironment_id,
			ClusterAccessGitopsEngineInstanceID: gitopsEngineInstance.Gitopsengineinstance_id,
		}

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

		application := &db.Application{
			ApplicationID:          "test-my-application-5",
			Name:                   "test'application",
			SpecField:              "{}",
			EngineInstanceInstID:   gitopsEngineInstance.Gitopsengineinstance_id,
			Managed_environment_id: managedEnvironment.Managedenvironment_id,
		}

		err = dbq.CheckedCreateApplication(ctx, application, clusterAccess.ClusterAccessUserID)
		Expect(err).To(BeNil())

		retrievedApplication := db.Application{ApplicationID: application.ApplicationID}

		err = dbq.GetApplicationById(ctx, &retrievedApplication)
		Expect(err).To(BeNil())
		Expect(application.Name).To(Equal(retrievedApplication.Name))
	})

	It("Should test TestClusterCredentialWrongInput", func() {
		err := db.SetupForTestingDBGinkgo()
		Expect(err).To(BeNil())
		dbq, err := db.NewUnsafePostgresDBQueries(true, true)
		Expect(err).To(BeNil())
		defer dbq.CloseDatabase()

		ctx := context.Background()

		clusterCredentials := db.ClusterCredentials{
			ClustercredentialsCredID: "test-cluster-creds-input",
			Host:                     "host'sInput'",
		}

		err = dbq.CreateClusterCredentials(ctx, &clusterCredentials)
		Expect(err).To(BeNil())
		retrievedClusterCredentials := &db.ClusterCredentials{
			ClustercredentialsCredID: clusterCredentials.ClustercredentialsCredID,
		}
		err = dbq.GetClusterCredentialsById(ctx, retrievedClusterCredentials)
		Expect(err).To(BeNil())
		Expect(clusterCredentials.Host).To(Equal(retrievedClusterCredentials.Host))
		Expect(err).To(BeNil())
	})

	It("Should test TestManagedEnviromentWrongInput", func() {
		err := db.SetupForTestingDBGinkgo()
		Expect(err).To(BeNil())
		dbq, err := db.NewUnsafePostgresDBQueries(true, true)
		Expect(err).To(BeNil())
		defer dbq.CloseDatabase()

		ctx := context.Background()

		clusterCredentials := db.ClusterCredentials{
			ClustercredentialsCredID:  "test-cluster-creds-test-1",
			Host:                      "host",
			KubeConfig:                "kube-config",
			KubeConfig_context:        "kube-config-context",
			ServiceAccountBearerToken: "serviceaccount_bearer_token",
			ServiceAccountNs:          "ServiceAccountNs",
		}

		err = dbq.CreateClusterCredentials(ctx, &clusterCredentials)
		Expect(err).To(BeNil())
		{
			managedEnvironment := db.ManagedEnvironment{
				Managedenvironment_id: "test-managed-env-1",
				ClusterCredentialsID:  clusterCredentials.ClustercredentialsCredID,
				Name:                  "test'env",
			}
			err = dbq.CreateManagedEnvironment(ctx, &managedEnvironment)
			Expect(err).To(BeNil())
			retrieveManagedEnv := &db.ManagedEnvironment{
				Managedenvironment_id: "test-managed-env-1",
			}
			err = dbq.GetManagedEnvironmentById(ctx, retrieveManagedEnv)
			Expect(err).To(BeNil())
			Expect(managedEnvironment.Name).To(Equal(retrieveManagedEnv.Name))
		}
		Expect(err).To(BeNil())
	})

	It("Should test TestClusterUserWrongInput", func() {
		err := db.SetupForTestingDBGinkgo()
		Expect(err).To(BeNil())
		dbq, err := db.NewUnsafePostgresDBQueries(true, true)
		Expect(err).To(BeNil())
		defer dbq.CloseDatabase()

		ctx := context.Background()

		user := &db.ClusterUser{
			ClusterUserID: "test-user-id",
			UserName:      "samyak'scluster",
		}
		err = dbq.CreateClusterUser(ctx, user)
		Expect(err).To(BeNil())

		retrieveUser := &db.ClusterUser{
			UserName: "samyak'scluster",
		}
		err = dbq.GetClusterUserByUsername(ctx, retrieveUser)
		Expect(err).To(BeNil())
	})

	It("Should test TestApplicationWrongInput", func() {
		err := db.SetupForTestingDBGinkgo()
		Expect(err).To(BeNil())
		dbq, err := db.NewUnsafePostgresDBQueries(true, true)
		Expect(err).To(BeNil())
		defer dbq.CloseDatabase()

		ctx := context.Background()

		var clusterUser = &db.ClusterUser{
			ClusterUserID: "test-user-wrong-application",
			UserName:      "test-user-wrong-application",
		}
		err = dbq.CreateClusterUser(ctx, clusterUser)
		Expect(err).To(BeNil())

		clusterCredentials := db.ClusterCredentials{
			ClustercredentialsCredID:  "test-cluster-creds-test-5",
			Host:                      "host",
			KubeConfig:                "kube-config",
			KubeConfig_context:        "kube-config-context",
			ServiceAccountBearerToken: "serviceaccount_bearer_token",
			ServiceAccountNs:          "ServiceAccountNs",
		}

		managedEnvironment := db.ManagedEnvironment{
			Managedenvironment_id: "test-managed-env-5",
			ClusterCredentialsID:  clusterCredentials.ClustercredentialsCredID,
			Name:                  "my env",
		}

		gitopsEngineCluster := db.GitopsEngineCluster{
			PrimaryKeyID:         "test-fake-cluster-5",
			ClusterCredentialsID: clusterCredentials.ClustercredentialsCredID,
		}

		gitopsEngineInstance := db.GitopsEngineInstance{
			Gitopsengineinstance_id: "test-fake-engine-instance-id",
			NamespaceName:           "test-fake-namespace",
			NamespaceUID:            "test-fake-namespace-5",
			EngineCluster_id:        gitopsEngineCluster.PrimaryKeyID,
		}

		clusterAccess := db.ClusterAccess{
			ClusterAccessUserID:                 clusterUser.ClusterUserID,
			ClusterAccessManagedEnvironmentID:   managedEnvironment.Managedenvironment_id,
			ClusterAccessGitopsEngineInstanceID: gitopsEngineInstance.Gitopsengineinstance_id,
		}

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

		application := &db.Application{
			ApplicationID:          "test-my-application-5",
			Name:                   "test'application",
			SpecField:              "{}",
			EngineInstanceInstID:   gitopsEngineInstance.Gitopsengineinstance_id,
			Managed_environment_id: managedEnvironment.Managedenvironment_id,
		}

		err = dbq.CheckedCreateApplication(ctx, application, clusterAccess.ClusterAccessUserID)
		Expect(err).To(BeNil())

		retrievedApplication := db.Application{ApplicationID: application.ApplicationID}

		err = dbq.GetApplicationById(ctx, &retrievedApplication)
		Expect(err).To(BeNil())
		Expect(application.Name).To(Equal(retrievedApplication.Name))
	})

})
