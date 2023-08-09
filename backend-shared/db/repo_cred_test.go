// db_test is a test suite for the db package.
// To run the tests: cd backend-shared; go test ./db -v -run TestDb
package db_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/db"
)

var _ = Describe("RepositoryCredentials Tests", func() {
	var (
		err                  error
		ctx                  context.Context
		clusterUser          *db.ClusterUser
		gitopsEngineInstance *db.GitopsEngineInstance
		dbq                  db.AllDatabaseQueries
	)

	When("When ClusterUser, ClusterCredentials, GitopsEngine and GitopsInstance exist", func() {
		BeforeEach(func() {
			By("Connecting to the database")
			err = db.SetupForTestingDBGinkgo()
			Expect(err).ToNot(HaveOccurred())

			dbq, err = db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).ToNot(HaveOccurred())

			ctx = context.Background()

			_, _, _, gitopsEngineInstance, _, err = db.CreateSampleData(dbq)
			Expect(err).ToNot(HaveOccurred())

			By("Satisfying the foreign key constraint 'fk_clusteruser_id'")
			// aka: ClusterUser.Clusteruser_id) required for 'repo_cred_user_id'
			clusterUser = &db.ClusterUser{
				Clusteruser_id: "test-repocred-user-id",
				User_name:      "test-repocred-user",
			}
			err = dbq.CreateClusterUser(ctx, clusterUser)
			Expect(err).ToNot(HaveOccurred())
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
			Expect(err).ToNot(HaveOccurred())

			By("Getting the RepositoryCredentials object from the database")
			fetch, err := dbq.GetRepositoryCredentialsByID(ctx, gitopsRepositoryCredentials.RepositoryCredentialsID)
			Expect(err).ToNot(HaveOccurred())
			Expect(fetch.Created_on.After(time.Now().Add(time.Minute*-5))).To(BeTrue(), "Created on should be within the last 5 minutes")
			fetch.Created_on = gitopsRepositoryCredentials.Created_on
			Expect(fetch).Should(Equal(gitopsRepositoryCredentials))

			By("Creating an identical RepositoryCredentials object should fail")
			gitopsRepositoryCredentials2 := db.RepositoryCredentials{
				RepositoryCredentialsID: "test-repo-cred-id",
				UserID:                  clusterUser.Clusteruser_id, // constrain 'fk_clusteruser_id'
				PrivateURL:              "https://test-private-url",
				AuthUsername:            "test-auth-username",
				AuthPassword:            "test-auth-password",
				AuthSSHKey:              "test-auth-ssh-key",
				SecretObj:               "test-secret-obj",
				EngineClusterID:         gitopsEngineInstance.Gitopsengineinstance_id, // constrain 'fk_gitopsengineinstance_id'
			}

			By("Inserting the identical RepositoryCredentials object to the database")
			err = dbq.CreateRepositoryCredentials(ctx, &gitopsRepositoryCredentials2)
			Expect(err).To(HaveOccurred())

			By("Updating the RepositoryCredentials object in the database")

			// copy the fetched object into a new one, so we can modify only the fields we want to update
			// and also keep a copy to compare the updated object with the original one.
			updatedCR := fetch
			updatedCR.AuthUsername = "updated-auth-username"
			updatedCR.AuthPassword = "updated-auth-password"
			updatedCR.AuthSSHKey = "updated-auth-ssh-key"
			updatedCR.SecretObj = "updated-secret-obj"

			err = dbq.UpdateRepositoryCredentials(ctx, &updatedCR)
			Expect(err).ToNot(HaveOccurred())

			By("Getting the updated RepositoryCredentials object from the database")
			fetchUpdated, err := dbq.GetRepositoryCredentialsByID(ctx, updatedCR.RepositoryCredentialsID)
			Expect(err).ToNot(HaveOccurred())
			Expect(fetch.Created_on.After(time.Now().Add(time.Minute*-5))).To(BeTrue(), "Created on should be within the last 5 minutes")
			fetchUpdated.Created_on = updatedCR.Created_on
			Expect(fetchUpdated).Should(Equal(updatedCR))

			By("Comparing the original and updated RepositoryCredentials objects")
			Expect(fetchUpdated.Created_on.After(time.Now().Add(time.Minute*-5))).To(BeTrue(), "Created on should be within the last 5 minutes")
			fetchUpdated.Created_on = updatedCR.Created_on
			Expect(fetch).ShouldNot(Equal(fetchUpdated))

			By("Deleting the RepositoryCredentials object from the database")
			rowsAffected, err := dbq.DeleteRepositoryCredentialsByID(ctx, gitopsRepositoryCredentials.RepositoryCredentialsID)
			Expect(err).ToNot(HaveOccurred())
			Expect(rowsAffected).Should(Equal(1))

			By("Getting the deleted RepositoryCredentials object from the database should fail")
			fetch, err = dbq.GetRepositoryCredentialsByID(ctx, gitopsRepositoryCredentials.RepositoryCredentialsID)
			Expect(err).Should(HaveOccurred())
			Expect(fetch).ShouldNot(Equal(gitopsRepositoryCredentials))

			By("Testing the hasEmptyValues function")

			By("Should create a UUID on the spot if Primary Key is null ")
			updatedCR.RepositoryCredentialsID = ""
			err = dbq.CreateRepositoryCredentials(ctx, &updatedCR)
			Expect(err).ShouldNot(HaveOccurred())

			updatedCR.RepositoryCredentialsID = "test-repo-cred-id" // reset the UserID to the original value

			By("Should return error if UserID is null")
			updatedCR.UserID = ""
			err = dbq.CreateRepositoryCredentials(ctx, &updatedCR)
			Expect(err).Should(HaveOccurred())
			expectedErr := "RepositoryCredentials.UserID is empty, but it shouldn't (notnull tag found: `repo_cred_user_id,notnull`)"
			Expect(err.Error()).Should(Equal(expectedErr))
			updatedCR.UserID = clusterUser.Clusteruser_id // reset the UserID to the original value

			By("Should return error if PrivateURL is null")
			updatedCR.PrivateURL = ""
			err = dbq.CreateRepositoryCredentials(ctx, &updatedCR)
			Expect(err).Should(HaveOccurred())
			expectedErr = "RepositoryCredentials.PrivateURL is empty, but it shouldn't (notnull tag found: `repo_cred_url,notnull`)"
			Expect(err.Error()).Should(Equal(expectedErr))
			updatedCR.PrivateURL = "https://test-private-url" // reset the PrivateURL to the original value

			By("Should return error if SecretObj is null")
			updatedCR.SecretObj = ""
			err = dbq.CreateRepositoryCredentials(ctx, &updatedCR)
			Expect(err).Should(HaveOccurred())
			expectedErr = "RepositoryCredentials.SecretObj is empty, but it shouldn't (notnull tag found: `repo_cred_secret,notnull`)"
			Expect(err.Error()).Should(Equal(expectedErr))
			updatedCR.SecretObj = "test-secret-obj" // reset the SecretObj to the original value

			By("Should return error if EngineClusterID is null")
			updatedCR.EngineClusterID = ""
			err = dbq.CreateRepositoryCredentials(ctx, &updatedCR)
			Expect(err).Should(HaveOccurred())
			expectedErr = "RepositoryCredentials.EngineClusterID is empty, but it shouldn't (notnull tag found: `repo_cred_engine_id,notnull`)"
			Expect(err.Error()).Should(Equal(expectedErr))
			updatedCR.EngineClusterID = gitopsEngineInstance.Gitopsengineinstance_id // reset the EngineClusterID to the original value
		})
	})

	Context("Test Dispose function for RepositoryCredentials", func() {
		var gitopsRepositoryCredentials db.RepositoryCredentials
		BeforeEach(func() {
			By("Connecting to the database")
			err = db.SetupForTestingDBGinkgo()
			Expect(err).ToNot(HaveOccurred())

			dbq, err = db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).ToNot(HaveOccurred())

			ctx = context.Background()

			_, _, _, gitopsEngineInstance, _, err = db.CreateSampleData(dbq)
			Expect(err).ToNot(HaveOccurred())

			By("Satisfying the foreign key constraint 'fk_clusteruser_id'")
			// aka: ClusterUser.Clusteruser_id) required for 'repo_cred_user_id'
			clusterUser = &db.ClusterUser{
				Clusteruser_id: "test-repocred-user-id",
				User_name:      "test-repocred-user",
			}
			err = dbq.CreateClusterUser(ctx, clusterUser)
			Expect(err).ToNot(HaveOccurred())

			By("Creating a RepositoryCredentials object")
			gitopsRepositoryCredentials = db.RepositoryCredentials{
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
			Expect(err).ToNot(HaveOccurred())
		})

		AfterEach(func() {
			defer dbq.CloseDatabase()
		})

		It("Should test Dispose function with missing database interface for RepositoryCredentials", func() {
			var dbq db.AllDatabaseQueries

			err := gitopsRepositoryCredentials.Dispose(ctx, dbq)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing database interface in RepositoryCredentials dispose"))

		})

		It("Should test Dispose function for RepositoryCredentials", func() {
			err := gitopsRepositoryCredentials.Dispose(context.Background(), dbq)
			Expect(err).ToNot(HaveOccurred())

			_, err = dbq.GetRepositoryCredentialsByID(ctx, gitopsRepositoryCredentials.RepositoryCredentialsID)
			Expect(err).To(HaveOccurred())

		})
	})
})
