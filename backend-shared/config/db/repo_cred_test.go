package db_test

import (
	"context"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
)

var _ = Describe("RepositoryCredentials Tests", func() {
	var (
		err                  error
		ctx                  context.Context
		clusterUser          *db.ClusterUser
		clusterCredentials   db.ClusterCredentials
		gitopsEngineCluster  db.GitopsEngineCluster
		gitopsEngineInstance db.GitopsEngineInstance
		dbq                  db.AllDatabaseQueries
	)

	When("When ClusterUser, ClusterCredentials, GitopsEngine and GitopsInstance exist", func() {
		BeforeEach(func() {
			// Connect to the database (the connection closes at AfterEach)
			err = db.SetupForTestingDBGinkgo()
			Expect(err).To(BeNil())

			dbq, err = db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).To(BeNil())

			ctx = context.Background()

			// Satisfy the foreign key constraint 'fk_clusteruser_id'
			// aka: ClusterUser.Clusteruser_id) required for 'repo_cred_user_id'
			clusterUser = &db.ClusterUser{
				Clusteruser_id: "test-repocred-user-id",
				User_name:      "test-repocred-user",
			}
			err = dbq.CreateClusterUser(ctx, clusterUser)
			Expect(err).To(BeNil())

			// Satisfy the foreign key constraint 'fk_gitopsengineinstance_id'
			// aka: GitOpsEngineInstance.Gitopsengineinstance_id) required for 'repo_cred_gitopsengineinstance_id'
			clusterCredentials = db.ClusterCredentials{
				Clustercredentials_cred_id:  "test-repocred-Clustercredentials_cred_id",
				Host:                        "test-repocred-host",
				Kube_config:                 "test-repocred-kube-config",
				Kube_config_context:         "test-repocred-kube-config-context",
				Serviceaccount_bearer_token: "test-repocred-serviceaccount_bearer_token",
				Serviceaccount_ns:           "test-repocred-Serviceaccount_ns",
			}

			err = dbq.CreateClusterCredentials(ctx, &clusterCredentials)
			Expect(err).To(BeNil())

			gitopsEngineCluster = db.GitopsEngineCluster{
				Gitopsenginecluster_id: "test-repocred-Gitopsenginecluster_id",
				Clustercredentials_id:  clusterCredentials.Clustercredentials_cred_id,
			}

			err = dbq.CreateGitopsEngineCluster(ctx, &gitopsEngineCluster)
			Expect(err).To(BeNil())

			gitopsEngineInstance = db.GitopsEngineInstance{
				Gitopsengineinstance_id: "test-repocred-Gitopsengineinstance_id",
				Namespace_name:          "test-repocred-Namespace_name",
				Namespace_uid:           "test-repocred-Namespace_uid",
				EngineCluster_id:        gitopsEngineCluster.Gitopsenginecluster_id,
			}

			err = dbq.CreateGitopsEngineInstance(ctx, &gitopsEngineInstance)
			Expect(err).To(BeNil())
		})
		AfterEach(func() {
			// Delete Cluster User, Cluster Credentials, Gitops Engine Cluster, Gitops Engine Instance.
			_, err = dbq.DeleteGitopsEngineInstanceById(ctx, gitopsEngineInstance.Gitopsengineinstance_id)
			Expect(err).To(BeNil())

			_, err = dbq.DeleteGitopsEngineClusterById(ctx, gitopsEngineCluster.Gitopsenginecluster_id)
			Expect(err).To(BeNil())

			_, err = dbq.DeleteClusterCredentialsById(ctx, clusterCredentials.Clustercredentials_cred_id)
			Expect(err).To(BeNil())

			_, err = dbq.DeleteClusterUserById(ctx, clusterUser.Clusteruser_id)
			Expect(err).To(BeNil())

			dbq.CloseDatabase() // Close the database connection.
		})

		It("it should create, update, get and delete RepositoryCredentials", func() {

			// Create a RepositoryCredentials object.
			gitopsRepositoryCredentials := db.RepositoryCredentials{
				PrimaryKeyID:    "test-repo-cred-id",
				UserID:          clusterUser.Clusteruser_id, // constrain 'fk_clusteruser_id'
				PrivateURL:      "https://test-private-url",
				AuthUsername:    "test-auth-username",
				AuthPassword:    "test-auth-password",
				AuthSSHKey:      "test-auth-ssh-key",
				SecretObj:       "test-secret-obj",
				EngineClusterID: gitopsEngineInstance.Gitopsengineinstance_id, // constrain 'fk_gitopsengineinstance_id'
			}

			// Insert the RepositoryCredentials to the database.
			err = dbq.CreateRepositoryCredentials(ctx, &gitopsRepositoryCredentials)
			Expect(err).To(BeNil())

			// Get the RepositoryCredentials from the database.
			fetch, err := dbq.GetRepositoryCredentialsByID(ctx, gitopsRepositoryCredentials.PrimaryKeyID)
			Expect(err).To(BeNil())
			Expect(fetch).Should(Equal(gitopsRepositoryCredentials))

			// Update the RepositoryCredentials in the database.
			updatedCR := db.RepositoryCredentials{
				PrimaryKeyID:    "test-repo-cred-id",
				UserID:          clusterUser.Clusteruser_id, // constrain 'fk_clusteruser_id'
				PrivateURL:      "https://test-private-url",
				AuthUsername:    "updated-auth-username",
				AuthPassword:    "updated-auth-password",
				AuthSSHKey:      "updated-auth-ssh-key",
				SecretObj:       "updated-secret-obj",
				EngineClusterID: gitopsEngineInstance.Gitopsengineinstance_id, // constrain 'fk_gitopsengineinstance_id'
			}

			// Update the RepositoryCredentials in the database.
			err = dbq.UpdateRepositoryCredentials(ctx, &updatedCR)
			Expect(err).To(BeNil())

			// Diff the two CRs (original and updated).
			Expect(gitopsRepositoryCredentials).ShouldNot(Equal(updatedCR))

			// Delete the RepositoryCredentials from the database.
			rowsAffected, err := dbq.DeleteRepositoryCredentialsByID(ctx, gitopsRepositoryCredentials.PrimaryKeyID)
			Expect(err).To(BeNil())
			Expect(rowsAffected).Should(Equal(1))

			fetch, err = dbq.GetRepositoryCredentialsByID(ctx, gitopsRepositoryCredentials.PrimaryKeyID)
			Expect(err).ShouldNot(BeNil())
			Expect(fetch).ShouldNot(Equal(gitopsRepositoryCredentials))

		})
	})
})
