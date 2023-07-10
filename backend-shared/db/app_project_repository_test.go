package db_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	db "github.com/redhat-appstudio/managed-gitops/backend-shared/db"
)

var _ = Describe("AppProjectRepository Test", func() {
	var seq = 101
	It("Should Create, Get, Update and Delete an AppProjectRepository", func() {
		err := db.SetupForTestingDBGinkgo()
		Expect(err).To(BeNil())

		ctx := context.Background()
		dbq, err := db.NewUnsafePostgresDBQueries(true, true)
		Expect(err).To(BeNil())
		defer dbq.CloseDatabase()

		_, _, _, gitopsEngineInstance, _, err := db.CreateSampleData(dbq)
		Expect(err).To(BeNil())

		var clusterUser = &db.ClusterUser{
			Clusteruser_id: "test-user-application",
			User_name:      "test-user-application",
		}
		err = dbq.CreateClusterUser(ctx, clusterUser)
		Expect(err).To(BeNil())

		repoCred := db.RepositoryCredentials{
			RepositoryCredentialsID: "test-repo-cred-id",
			UserID:                  clusterUser.Clusteruser_id,
			PrivateURL:              "https://test-private-url",
			AuthUsername:            "test-auth-username",
			AuthPassword:            "test-auth-password",
			AuthSSHKey:              "test-auth-ssh-key",
			SecretObj:               "test-secret-obj",
			EngineClusterID:         gitopsEngineInstance.Gitopsengineinstance_id,
		}

		By("Inserting the RepositoryCredentials object to the database")
		err = dbq.CreateRepositoryCredentials(ctx, &repoCred)
		Expect(err).To(BeNil())

		By("Verify whether AppProjectRepository is created")
		appProjectRepository := db.AppProjectRepository{
			AppprojectRepositoryID:  "test-app-project-repository",
			Clusteruser_id:          clusterUser.Clusteruser_id,
			RepositorycredentialsID: repoCred.RepositoryCredentialsID,
			RepoURL:                 repoCred.PrivateURL,
			SeqID:                   int64(seq),
		}

		err = dbq.CreateAppProjectRepository(ctx, &appProjectRepository)
		Expect(err).To(BeNil())

		By("Verify whether AppProjectRepository is retrieved")
		appProjectRepositoryget := db.AppProjectRepository{
			Clusteruser_id: clusterUser.Clusteruser_id,
			RepoURL:        repoCred.PrivateURL,
		}

		err = dbq.GetAppProjectRepositoryByClusterUserAndRepoURL(ctx, &appProjectRepositoryget)
		Expect(err).To(BeNil())
		Expect(appProjectRepository).Should(Equal(appProjectRepositoryget))

		By("Verify CountAppProjectRepositoryByClusterUserID")
		appProjectRepositoryCount, err := dbq.CountAppProjectRepositoryByClusterUserID(ctx, &appProjectRepositoryget)
		Expect(err).To(BeNil())
		Expect(appProjectRepositoryCount).To(Equal(1))

		By("Verify whether AppProjectRepository is deleted")
		rowsAffected, err := dbq.DeleteAppProjectRepositoryByRepoCredId(ctx, &appProjectRepository)
		Expect(err).To(BeNil())
		Expect(rowsAffected).Should(Equal(1))

		err = dbq.GetAppProjectRepositoryByClusterUserAndRepoURL(ctx, &appProjectRepository)
		Expect(true).To(Equal(db.IsResultNotFoundError(err)))

		appProjectRepositoryget = db.AppProjectRepository{
			AppprojectRepositoryID: "does-not-exist",
		}
		err = dbq.GetAppProjectRepositoryByClusterUserAndRepoURL(ctx, &appProjectRepositoryget)
		Expect(true).To(Equal(db.IsResultNotFoundError(err)))

		By("Verify whether AppProjectRepository is created")
		appProjectRepository1 := db.AppProjectRepository{
			AppprojectRepositoryID:  "test-app-project-repository-1",
			Clusteruser_id:          clusterUser.Clusteruser_id,
			RepositorycredentialsID: repoCred.RepositoryCredentialsID,
			RepoURL:                 repoCred.PrivateURL,
			SeqID:                   int64(seq),
		}

		err = dbq.CreateAppProjectRepository(ctx, &appProjectRepository1)
		Expect(err).To(BeNil())

		err = dbq.GetAppProjectRepositoryByClusterUserAndRepoURL(ctx, &appProjectRepository1)
		Expect(err).To(BeNil())

		By("Verify whether AppProjectRepository is deleted based on clusteruser_is")
		rowsAffected, err = dbq.DeleteAppProjectRepositoryByClusterUserAndRepoURL(ctx, &appProjectRepository1)
		Expect(err).To(BeNil())
		Expect(rowsAffected).Should(Equal(1))

	})

})
