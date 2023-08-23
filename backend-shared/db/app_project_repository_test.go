package db_test

import (
	"context"
	"fmt"
	"strconv"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	db "github.com/redhat-appstudio/managed-gitops/backend-shared/db"
)

var _ = Describe("AppProjectRepository Test", func() {
	var seq = 101

	var (
		ctx context.Context
		dbq db.AllDatabaseQueries
	)

	BeforeEach(func() {
		err := db.SetupForTestingDBGinkgo()
		Expect(err).ToNot(HaveOccurred())

		ctx = context.Background()
		dbq, err = db.NewUnsafePostgresDBQueries(true, true)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		dbq.CloseDatabase()
	})

	It("Should Create, Get, Update and Delete an AppProjectRepository", func() {
		_, _, _, gitopsEngineInstance, _, err := db.CreateSampleData(dbq)
		Expect(err).ToNot(HaveOccurred())

		var clusterUser = &db.ClusterUser{
			Clusteruser_id: "test-user-application",
			User_name:      "test-user-application",
		}
		err = dbq.CreateClusterUser(ctx, clusterUser)
		Expect(err).ToNot(HaveOccurred())

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
		Expect(err).ToNot(HaveOccurred())

		By("Verify whether AppProjectRepository is created")
		appProjectRepository := db.AppProjectRepository{
			AppprojectRepositoryID: "test-app-project-repository",
			Clusteruser_id:         clusterUser.Clusteruser_id,
			RepoURL:                repoCred.PrivateURL,
			SeqID:                  int64(seq),
		}

		err = dbq.CreateAppProjectRepository(ctx, &appProjectRepository)
		Expect(err).ToNot(HaveOccurred())

		By("Verify whether AppProjectRepository is retrieved")
		appProjectRepositoryget := db.AppProjectRepository{
			Clusteruser_id: clusterUser.Clusteruser_id,
			RepoURL:        repoCred.PrivateURL,
		}

		err = dbq.GetAppProjectRepositoryByClusterUserAndRepoURL(ctx, &appProjectRepositoryget)
		Expect(err).ToNot(HaveOccurred())
		Expect(appProjectRepository).Should(Equal(appProjectRepositoryget))

		By("Verify CountAppProjectRepositoryByClusterUserID")
		appProjectRepositoryCount, err := dbq.CountAppProjectRepositoryByClusterUserID(ctx, &appProjectRepositoryget)
		Expect(err).ToNot(HaveOccurred())
		Expect(appProjectRepositoryCount).To(Equal(1))

		By("Verify whether AppProjectRepository is deleted")
		rowsAffected, err := dbq.DeleteAppProjectRepositoryByClusterUserAndRepoURL(ctx, &appProjectRepository)
		Expect(err).ToNot(HaveOccurred())
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
			AppprojectRepositoryID: "test-app-project-repository-1",
			Clusteruser_id:         clusterUser.Clusteruser_id,
			RepoURL:                repoCred.PrivateURL,
			SeqID:                  int64(seq),
		}

		err = dbq.CreateAppProjectRepository(ctx, &appProjectRepository1)
		Expect(err).ToNot(HaveOccurred())

		err = dbq.GetAppProjectRepositoryByClusterUserAndRepoURL(ctx, &appProjectRepository1)
		Expect(err).ToNot(HaveOccurred())

		By("Verify whether AppProjectRepository is deleted based on clusteruser_is")
		rowsAffected, err = dbq.DeleteAppProjectRepositoryByClusterUserAndRepoURL(ctx, &appProjectRepository1)
		Expect(err).ToNot(HaveOccurred())
		Expect(rowsAffected).Should(Equal(1))

	})

	Context("Test ListAppProjectRepositoryByClusterUserId function", func() {
		It("should return a list of AppProjectRepositories for a given clusterUser", func() {
			_, _, _, gitopsEngineInstance, _, err := db.CreateSampleData(dbq)
			Expect(err).ToNot(HaveOccurred())

			var clusterUser = &db.ClusterUser{
				Clusteruser_id: "test-user-application",
				User_name:      "test-user-application",
			}
			err = dbq.CreateClusterUser(ctx, clusterUser)
			Expect(err).ToNot(HaveOccurred())

			By("create AppProjectRepositories for the given clusterUsers")
			expectedRows := 4
			appProjectRepository := db.AppProjectRepository{
				AppprojectRepositoryID: "test-app-project-repository",
				Clusteruser_id:         clusterUser.Clusteruser_id,
				SeqID:                  int64(seq),
			}
			appProjectRepoMap := map[string]db.AppProjectRepository{}

			for i := 1; i <= expectedRows; i++ {
				repoCred := db.RepositoryCredentials{
					RepositoryCredentialsID: fmt.Sprintf("test-repo-cred-id-%d", i),
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
				Expect(err).ToNot(HaveOccurred())

				appProjectRepository.AppprojectRepositoryID = fmt.Sprintf("test-app-project-repository-%d", i)
				appProjectRepository.RepoURL = repoCred.PrivateURL + strconv.Itoa(i)
				err = dbq.CreateAppProjectRepository(ctx, &appProjectRepository)
				Expect(err).ToNot(HaveOccurred())

				appProjectRepoMap[appProjectRepository.AppprojectRepositoryID] = appProjectRepository
			}

			By("verify if we can list all the AppProjectRepositories for a given user")
			appProjectRepositories := []db.AppProjectRepository{}
			err = dbq.ListAppProjectRepositoryByClusterUserId(ctx, clusterUser.Clusteruser_id, &appProjectRepositories)
			Expect(err).ToNot(HaveOccurred())
			Expect(appProjectRepositories).To(HaveLen(expectedRows))

			for _, appRepo := range appProjectRepositories {
				expectedAppRepo, found := appProjectRepoMap[appRepo.AppprojectRepositoryID]
				Expect(found).To(BeTrue())
				Expect(expectedAppRepo).To(Equal(appRepo))
			}
		})

		It("should return an error if the clusterUser ID is empty", func() {
			appProjectRepositories := []db.AppProjectRepository{}
			err := dbq.ListAppProjectRepositoryByClusterUserId(ctx, "", &appProjectRepositories)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("primary key is empty"))
		})

		It("should return an empty slice if there are rows with the given clusterUserID", func() {
			appProjectRepositories := []db.AppProjectRepository{}
			err := dbq.ListAppProjectRepositoryByClusterUserId(ctx, "invalid-ID", &appProjectRepositories)
			Expect(err).ToNot(HaveOccurred())
			Expect(appProjectRepositories).To(BeEmpty())
		})
	})

})
