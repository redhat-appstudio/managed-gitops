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
			By("Connecting to the database")
			err = db.SetupForTestingDBGinkgo()
			Expect(err).To(BeNil())

			dbq, err = db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).To(BeNil())

			ctx = context.Background()

			By("Satisfying the foreign key constraint 'fk_clusteruser_id'")
			// aka: ClusterUser.Clusteruser_id) required for 'repo_cred_user_id'
			clusterUser = &db.ClusterUser{
				Clusteruser_id: "test-repocred-user-id",
				User_name:      "test-repocred-user",
			}
			err = dbq.CreateClusterUser(ctx, clusterUser)
			Expect(err).To(BeNil())

			By("Satisfying the foreign key constraint 'fk_gitopsengineinstance_id'")
			// aka: GitOpsEngineInstance.Gitopsengineinstance_id) required for 'repo_cred_gitopsengineinstance_id'
			clusterCredentials = db.ClusterCredentials{
				Clustercredentials_cred_id:  "test-repocred-Clustercredentials_cred_id",
				Host:                        "test-repocred-host",
				Kube_config:                 "test-repocred-kube-config",
				Kube_config_context:         "test-repocred-kube-config-context",
				Serviceaccount_bearer_token: "test-repocred-serviceaccount_bearer_token",
				Serviceaccount_ns:           "test-repocred-Serviceaccount_ns",
			}

			By("Creating clusterCredentials as a pre-requisite for GitopsEngineCluster")
			err = dbq.CreateClusterCredentials(ctx, &clusterCredentials)
			Expect(err).To(BeNil())

			gitopsEngineCluster = db.GitopsEngineCluster{
				Gitopsenginecluster_id: "test-repocred-Gitopsenginecluster_id",
				Clustercredentials_id:  clusterCredentials.Clustercredentials_cred_id,
			}

			By("Creating gitopsEngineCluster as a pre-requisite for GitopsEngineInstance")
			err = dbq.CreateGitopsEngineCluster(ctx, &gitopsEngineCluster)
			Expect(err).To(BeNil())

			gitopsEngineInstance = db.GitopsEngineInstance{
				Gitopsengineinstance_id: "test-repocred-Gitopsengineinstance_id",
				Namespace_name:          "test-repocred-Namespace_name",
				Namespace_uid:           "test-repocred-Namespace_uid",
				EngineCluster_id:        gitopsEngineCluster.Gitopsenginecluster_id,
			}

			By("Creating gitopsEngineInstance as a pre-requisite for RepositoryCredentials")
			err = dbq.CreateGitopsEngineInstance(ctx, &gitopsEngineInstance)
			Expect(err).To(BeNil())
		})
		AfterEach(func() {
			By("Closing the database connection")
			dbq.CloseDatabase() // Close the database connection.
		})

		It("it should create, update, get and delete RepositoryCredentials", func() {

			By("Creating a RepositoryCredentials object")
			gitopsRepositoryCredentials := db.RepositoryCredentials{
				RepositoryCredentialsID: "test-repo-cred-id",
				UserID:                  clusterUser.Clusteruser_id, // constrain 'fk_clusteruser_id'
				PrivateURL:              "https://test-private-url",
				AuthUsername:            "test-auth-username",
				AuthPassword:            "test-auth-password",
				AuthSSHKey:              "test-auth-ssh-key",
				SecretObj:               "test-secret-obj",
				EngineClusterID:         gitopsEngineInstance.Gitopsengineinstance_id, // constrain 'fk_gitopsengineinstance_id'
			}

			By("Inserting the RepositoryCredentials object to the database")
			err = dbq.CreateRepositoryCredentials(ctx, &gitopsRepositoryCredentials)
			Expect(err).To(BeNil())

			By("Getting the RepositoryCredentials object from the database")
			fetch, err := dbq.GetRepositoryCredentialsByID(ctx, gitopsRepositoryCredentials.RepositoryCredentialsID)
			Expect(err).To(BeNil())
			Expect(fetch).Should(Equal(gitopsRepositoryCredentials))

			By("Updating the RepositoryCredentials object in the database")

			// copy the fetched object into a new one, so we can modify only the fields we want to update
			// and also keep a copy to compare the updated object with the original one.
			updatedCR := fetch
			updatedCR.AuthUsername = "updated-auth-username"
			updatedCR.AuthPassword = "updated-auth-password"
			updatedCR.AuthSSHKey = "updated-auth-ssh-key"
			updatedCR.SecretObj = "updated-secret-obj"

			err = dbq.UpdateRepositoryCredentials(ctx, &updatedCR)
			Expect(err).To(BeNil())

			By("Getting the updated RepositoryCredentials object from the database")
			fetchUpdated, err := dbq.GetRepositoryCredentialsByID(ctx, updatedCR.RepositoryCredentialsID)
			Expect(err).To(BeNil())
			Expect(fetchUpdated).Should(Equal(updatedCR))

			By("Comparing the original and updated RepositoryCredentials objects")
			Expect(fetch).ShouldNot(Equal(fetchUpdated))

			By("Deleting the RepositoryCredentials object from the database")
			rowsAffected, err := dbq.DeleteRepositoryCredentialsByID(ctx, gitopsRepositoryCredentials.RepositoryCredentialsID)
			Expect(err).To(BeNil())
			Expect(rowsAffected).Should(Equal(1))

			By("Getting the deleted RepositoryCredentials object from the database should fail")
			fetch, err = dbq.GetRepositoryCredentialsByID(ctx, gitopsRepositoryCredentials.RepositoryCredentialsID)
			Expect(err).ShouldNot(BeNil())
			Expect(fetch).ShouldNot(Equal(gitopsRepositoryCredentials))

		})
	})
})
