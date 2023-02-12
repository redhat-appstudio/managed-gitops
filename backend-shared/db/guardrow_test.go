package db_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/db"
)

var _ = Describe("Test to verify update/delete operations are not globally scoped", func() {
	Context("It creates database unit tests which guard against missing WHERE clauses of UPDATE/DELETE operations to the database ", func() {

		It("Should test guard row against delete for ApiCRtoDBmapping", func() {
			err := db.SetupForTestingDBGinkgo()
			Expect(err).To(BeNil())

			ctx := context.Background()
			dbq, err := db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).To(BeNil())
			defer dbq.CloseDatabase()

			ApicrtodatabasemappingFirst := db.APICRToDatabaseMapping{
				APIResourceType:      db.APICRToDatabaseMapping_ResourceType_GitOpsDeploymentSyncRun,
				APIResourceUID:       "test-k8s-uid",
				APIResourceName:      "test-k8s-name",
				APIResourceNamespace: "test-k8s-namespace",
				NamespaceUID:         "test-namespace-uid",
				DBRelationType:       db.APICRToDatabaseMapping_DBRelationType_SyncOperation,
				DBRelationKey:        "test-key",
			}

			err = dbq.CreateAPICRToDatabaseMapping(ctx, &ApicrtodatabasemappingFirst)
			Expect(err).To(BeNil())

			ApicrtodatabasemappingSecond := db.APICRToDatabaseMapping{
				APIResourceType:      "test-GitOpsDeployment",
				APIResourceUID:       "test-k8s-uid-second",
				APIResourceName:      "test-k8s-name",
				APIResourceNamespace: "test-k8s-namespace",
				NamespaceUID:         "test-namespace-uid",
				DBRelationType:       "test-sync-operation",
				DBRelationKey:        "test-key-second",
			}
			err = dbq.CreateAPICRToDatabaseMapping(ctx, &ApicrtodatabasemappingSecond)
			Expect(err).To(BeNil())

			rowsAffected, err := dbq.DeleteAPICRToDatabaseMapping(ctx, &ApicrtodatabasemappingSecond)
			Expect(err).To(BeNil())
			Expect(rowsAffected).To(Equal((1)))

			err = dbq.GetDatabaseMappingForAPICR(ctx, &ApicrtodatabasemappingFirst)
			Expect(err).To(BeNil())

			err = dbq.GetDatabaseMappingForAPICR(ctx, &ApicrtodatabasemappingSecond)
			Expect(true).To(Equal(db.IsResultNotFoundError(err)))

		})

		It("Should test guard row against update and delete on application", func() {
			err := db.SetupForTestingDBGinkgo()
			Expect(err).To(BeNil())

			ctx := context.Background()
			dbq, err := db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).To(BeNil())
			defer dbq.CloseDatabase()

			_, managedEnvironment, _, gitopsEngineInstance, _, err := db.CreateSampleData(dbq)
			Expect(err).To(BeNil())

			applicationFirst := db.Application{
				ApplicationID:          "test-my-application-1",
				Name:                   "my-application",
				SpecField:              "{}",
				EngineInstanceInstID:   gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id: managedEnvironment.Managedenvironment_id,
			}

			err = dbq.CreateApplication(ctx, &applicationFirst)
			Expect(err).To(BeNil())

			applicationSecond := db.Application{
				ApplicationID:          "test-my-application-2",
				Name:                   "my-application",
				SpecField:              "{}",
				EngineInstanceInstID:   gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id: managedEnvironment.Managedenvironment_id,
			}
			err = dbq.CreateApplication(ctx, &applicationSecond)
			Expect(err).To(BeNil())

			applicationSecond = db.Application{
				ApplicationID:          applicationSecond.ApplicationID,
				Name:                   "test-application-update",
				SpecField:              "{}",
				EngineInstanceInstID:   gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id: managedEnvironment.Managedenvironment_id,
				SeqID:                  applicationSecond.SeqID,
				CreatedOn:              applicationFirst.CreatedOn,
			}

			err = dbq.UpdateApplication(ctx, &applicationSecond)
			Expect(err).To(BeNil())
			err = dbq.GetApplicationById(ctx, &applicationFirst)
			Expect(err).To(BeNil())
			err = dbq.GetApplicationById(ctx, &applicationSecond)
			Expect(err).To(BeNil())
			Expect(applicationSecond.Name).Should(Equal("test-application-update"))
			Expect(applicationFirst.Name).ShouldNot(Equal(applicationSecond.Name))

			rowsAffected, err := dbq.DeleteApplicationById(ctx, applicationSecond.ApplicationID)
			Expect(err).To(BeNil())
			Expect(rowsAffected).Should(Equal(1))

			err = dbq.GetApplicationById(ctx, &applicationFirst)
			Expect(err).To(BeNil())

			err = dbq.GetApplicationById(ctx, &applicationSecond)
			Expect(true).To(Equal(db.IsResultNotFoundError(err)))

		})

		It("Should test guard row against update and delete for applicationstates", func() {
			err := db.SetupForTestingDBGinkgo()
			Expect(err).To(BeNil())

			ctx := context.Background()
			dbq, err := db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).To(BeNil())
			defer dbq.CloseDatabase()

			_, managedEnvironment, _, gitopsEngineInstance, _, err := db.CreateSampleData(dbq)
			Expect(err).To(BeNil())

			applicationFirst := db.Application{
				ApplicationID:          "test-my-application-1",
				Name:                   "my-application",
				SpecField:              "{}",
				EngineInstanceInstID:   gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id: managedEnvironment.Managedenvironment_id,
			}

			err = dbq.CreateApplication(ctx, &applicationFirst)
			Expect(err).To(BeNil())

			applicationStateFirst := &db.ApplicationState{
				Applicationstate_application_id: applicationFirst.ApplicationID,
				Health:                          "Progressing",
				SyncStatus:                      "Unknown",
				Resources:                       make([]byte, 10),
				ReconciledState:                 "test-reconciledState",
				SyncError:                       "test-sync-error",
			}

			err = dbq.CreateApplicationState(ctx, applicationStateFirst)
			Expect(err).To(BeNil())

			applicationSecond := db.Application{
				ApplicationID:          "test-my-application-2",
				Name:                   "my-application",
				SpecField:              "{}",
				EngineInstanceInstID:   gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id: managedEnvironment.Managedenvironment_id,
			}
			err = dbq.CreateApplication(ctx, &applicationSecond)
			Expect(err).To(BeNil())

			applicationStateSecond := &db.ApplicationState{
				Applicationstate_application_id: applicationSecond.ApplicationID,
				Health:                          "Progressing",
				SyncStatus:                      "Unknown",
				Resources:                       make([]byte, 10),
				ReconciledState:                 "test-reconciledState",
				SyncError:                       "test-sync-error",
			}

			err = dbq.CreateApplicationState(ctx, applicationStateSecond)
			Expect(err).To(BeNil())
			applicationStateSecond = &db.ApplicationState{
				Applicationstate_application_id: applicationSecond.ApplicationID,
				Health:                          "Progressing",
				SyncStatus:                      "Sync",
				Resources:                       make([]byte, 10),
				ReconciledState:                 "test-reconciledState",
				SyncError:                       "test-sync-error",
			}

			err = dbq.UpdateApplicationState(ctx, applicationStateSecond)
			Expect(err).To(BeNil())
			err = dbq.GetApplicationStateById(ctx, applicationStateFirst)
			Expect(err).To(BeNil())
			err = dbq.GetApplicationStateById(ctx, applicationStateSecond)
			Expect(err).To(BeNil())

			Expect(applicationStateSecond.SyncStatus).Should(Equal("Sync"))
			Expect(applicationStateFirst.SyncStatus).ShouldNot(Equal(applicationStateSecond.SyncStatus))

			rowsAffected, err := dbq.DeleteApplicationStateById(ctx, applicationStateSecond.Applicationstate_application_id)
			Expect(err).To(BeNil())
			Expect(rowsAffected).Should(Equal(1))

			err = dbq.GetApplicationStateById(ctx, applicationStateFirst)
			Expect(err).To(BeNil())

			err = dbq.GetApplicationStateById(ctx, applicationStateSecond)
			Expect(true).To(Equal(db.IsResultNotFoundError(err)))

		})

		It("Should test guard row against delete for clusteraccess", func() {
			err := db.SetupForTestingDBGinkgo()
			Expect(err).To(BeNil())

			ctx := context.Background()
			dbq, err := db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).To(BeNil())
			defer dbq.CloseDatabase()

			var clusterUser = &db.ClusterUser{
				ClusterUserID: "test-user-1",
				UserName:      "test-user-1",
			}
			err = dbq.CreateClusterUser(ctx, clusterUser)
			Expect(err).To(BeNil())

			clusterCredentialsFirst := db.ClusterCredentials{
				ClustercredentialsCredID:  "test-cluster-creds-test-1",
				Host:                      "host",
				KubeConfig:                "kube-config",
				KubeConfig_context:        "kube-config-context",
				ServiceAccountBearerToken: "serviceaccount_bearer_token",
				ServiceAccountNs:          "ServiceAccountNs",
			}

			managedEnvironmentFirst := db.ManagedEnvironment{
				Managedenvironment_id: "test-managed-env-1",
				ClusterCredentialsID:  clusterCredentialsFirst.ClustercredentialsCredID,
				Name:                  "my env",
			}

			gitopsEngineClusterFirst := db.GitopsEngineCluster{
				PrimaryKeyID:         "test-fake-cluster-1",
				ClusterCredentialsID: clusterCredentialsFirst.ClustercredentialsCredID,
			}

			gitopsEngineInstanceFirst := db.GitopsEngineInstance{
				Gitopsengineinstance_id: "test-fake-engine-instance-id-1",
				NamespaceName:           "test-fake-namespace",
				NamespaceUID:            "test-fake-namespace-5",
				EngineCluster_id:        gitopsEngineClusterFirst.PrimaryKeyID,
			}

			clusterAccessFirst := db.ClusterAccess{
				ClusterAccessUserID:                 clusterUser.ClusterUserID,
				ClusterAccessManagedEnvironmentID:   managedEnvironmentFirst.Managedenvironment_id,
				ClusterAccessGitopsEngineInstanceID: gitopsEngineInstanceFirst.Gitopsengineinstance_id,
			}

			err = dbq.CreateClusterCredentials(ctx, &clusterCredentialsFirst)
			Expect(err).To(BeNil())

			err = dbq.CreateManagedEnvironment(ctx, &managedEnvironmentFirst)
			Expect(err).To(BeNil())

			err = dbq.CreateGitopsEngineCluster(ctx, &gitopsEngineClusterFirst)
			Expect(err).To(BeNil())

			err = dbq.CreateGitopsEngineInstance(ctx, &gitopsEngineInstanceFirst)
			Expect(err).To(BeNil())

			err = dbq.CreateClusterAccess(ctx, &clusterAccessFirst)
			Expect(err).To(BeNil())

			clusterUser = &db.ClusterUser{
				ClusterUserID: "test-user-2",
				UserName:      "test-user-2",
			}
			err = dbq.CreateClusterUser(ctx, clusterUser)
			Expect(err).To(BeNil())

			clusterCredentialsSecond := db.ClusterCredentials{
				ClustercredentialsCredID:  "test-cluster-creds-test-2",
				Host:                      "host",
				KubeConfig:                "kube-config",
				KubeConfig_context:        "kube-config-context",
				ServiceAccountBearerToken: "serviceaccount_bearer_token",
				ServiceAccountNs:          "ServiceAccountNs",
			}

			managedEnvironmentSecond := db.ManagedEnvironment{
				Managedenvironment_id: "test-managed-env-2",
				ClusterCredentialsID:  clusterCredentialsSecond.ClustercredentialsCredID,
				Name:                  "my env",
			}

			gitopsEngineClusterSecond := db.GitopsEngineCluster{
				PrimaryKeyID:         "test-fake-cluster-2",
				ClusterCredentialsID: clusterCredentialsSecond.ClustercredentialsCredID,
			}

			gitopsEngineInstanceSecond := db.GitopsEngineInstance{
				Gitopsengineinstance_id: "test-fake-engine-instance-id-2",
				NamespaceName:           "test-fake-namespace",
				NamespaceUID:            "test-fake-namespace-5",
				EngineCluster_id:        gitopsEngineClusterSecond.PrimaryKeyID,
			}

			clusterAccessSecond := db.ClusterAccess{
				ClusterAccessUserID:                 clusterUser.ClusterUserID,
				ClusterAccessManagedEnvironmentID:   managedEnvironmentSecond.Managedenvironment_id,
				ClusterAccessGitopsEngineInstanceID: gitopsEngineInstanceSecond.Gitopsengineinstance_id,
			}

			err = dbq.CreateClusterCredentials(ctx, &clusterCredentialsSecond)
			Expect(err).To(BeNil())

			err = dbq.CreateManagedEnvironment(ctx, &managedEnvironmentSecond)
			Expect(err).To(BeNil())

			err = dbq.CreateGitopsEngineCluster(ctx, &gitopsEngineClusterSecond)
			Expect(err).To(BeNil())

			err = dbq.CreateGitopsEngineInstance(ctx, &gitopsEngineInstanceSecond)
			Expect(err).To(BeNil())

			err = dbq.CreateClusterAccess(ctx, &clusterAccessSecond)
			Expect(err).To(BeNil())

			affectedRows, err := dbq.DeleteClusterAccessById(ctx, clusterAccessSecond.ClusterAccessUserID, clusterAccessSecond.ClusterAccessManagedEnvironmentID, clusterAccessSecond.ClusterAccessGitopsEngineInstanceID)
			Expect(err).To(BeNil())
			Expect(affectedRows).To(Equal(1))

			err = dbq.GetClusterAccessByPrimaryKey(ctx, &clusterAccessFirst)
			Expect(err).To(BeNil())

			err = dbq.GetClusterAccessByPrimaryKey(ctx, &clusterAccessSecond)
			Expect(true).To(Equal(db.IsResultNotFoundError(err)))

		})

		It("Should test guard row against delete for clustercredentials", func() {
			err := db.SetupForTestingDBGinkgo()
			Expect(err).To(BeNil())

			ctx := context.Background()
			dbq, err := db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).To(BeNil())
			defer dbq.CloseDatabase()

			clusterCredFirst := db.ClusterCredentials{
				Host:                      "test-host",
				KubeConfig:                "test-kube_config",
				KubeConfig_context:        "test-kube_config_context",
				ServiceAccountBearerToken: "test-serviceaccount_bearer_token",
				ServiceAccountNs:          "test-serviceaccount_ns",
			}
			err = dbq.CreateClusterCredentials(ctx, &clusterCredFirst)
			Expect(err).To(BeNil())

			clusterCredSecond := db.ClusterCredentials{
				Host:                      "test-host",
				KubeConfig:                "test-kube_config",
				KubeConfig_context:        "test-kube_config_context",
				ServiceAccountBearerToken: "test-serviceaccount_bearer_token",
				ServiceAccountNs:          "test-serviceaccount_ns",
			}
			err = dbq.CreateClusterCredentials(ctx, &clusterCredSecond)
			Expect(err).To(BeNil())

			rowsAffected, err := dbq.DeleteClusterCredentialsById(ctx, clusterCredSecond.ClustercredentialsCredID)
			Expect(err).To(BeNil())
			Expect(rowsAffected).Should(Equal(1))

			err = dbq.GetClusterCredentialsById(ctx, &clusterCredFirst)
			Expect(err).To(BeNil())

			err = dbq.GetClusterCredentialsById(ctx, &clusterCredSecond)
			Expect(true).To(Equal(db.IsResultNotFoundError(err)))

		})

		It("Should test guard row against delete for clusteruser", func() {
			err := db.SetupForTestingDBGinkgo()
			Expect(err).To(BeNil())

			ctx := context.Background()
			dbq, err := db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).To(BeNil())
			defer dbq.CloseDatabase()

			userfirst := &db.ClusterUser{
				ClusterUserID: "test-user-id-1",
				UserName:      "test-user-1",
			}
			err = dbq.CreateClusterUser(ctx, userfirst)
			Expect(err).To(BeNil())

			usersecond := &db.ClusterUser{
				ClusterUserID: "test-user-id-2",
				UserName:      "test-user-2",
			}
			err = dbq.CreateClusterUser(ctx, usersecond)
			Expect(err).To(BeNil())

			rowsAffected, err := dbq.DeleteClusterUserById(ctx, usersecond.ClusterUserID)
			Expect(err).To(BeNil())
			Expect(rowsAffected).Should(Equal(1))

			err = dbq.GetClusterUserById(ctx, userfirst)
			Expect(err).To(BeNil())

			err = dbq.GetClusterUserById(ctx, usersecond)
			Expect(true).To(Equal(db.IsResultNotFoundError(err)))

		})

		It("Should test guard row against delete for deploymenttoapplicationmapping", func() {
			err := db.SetupForTestingDBGinkgo()
			Expect(err).To(BeNil())

			ctx := context.Background()
			dbq, err := db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).To(BeNil())
			defer dbq.CloseDatabase()

			_, managedEnvironment, _, gitopsEngineInstance, _, err := db.CreateSampleData(dbq)
			Expect(err).To(BeNil())

			applicationFirst := db.Application{
				ApplicationID:          "test-my-application-1",
				Name:                   "my-application",
				SpecField:              "{}",
				EngineInstanceInstID:   gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id: managedEnvironment.Managedenvironment_id,
			}

			err = dbq.CreateApplication(ctx, &applicationFirst)
			Expect(err).To(BeNil())

			deploymentToApplicationMappingfirst := &db.DeploymentToApplicationMapping{
				Deploymenttoapplicationmapping_uid_id: "test-" + generateUuid(),
				ApplicationID:                         applicationFirst.ApplicationID,
				DeploymentName:                        "test-deployment",
				DeploymentNamespace:                   "test-namespace",
				NamespaceUID:                          "demo-namespace",
			}

			err = dbq.CreateDeploymentToApplicationMapping(ctx, deploymentToApplicationMappingfirst)
			Expect(err).To(BeNil())

			applicationSecond := db.Application{
				ApplicationID:          "test-my-application-2",
				Name:                   "my-application",
				SpecField:              "{}",
				EngineInstanceInstID:   gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id: managedEnvironment.Managedenvironment_id,
			}
			err = dbq.CreateApplication(ctx, &applicationSecond)
			Expect(err).To(BeNil())

			deploymentToApplicationMappingsecond := &db.DeploymentToApplicationMapping{
				Deploymenttoapplicationmapping_uid_id: "test-" + generateUuid(),
				ApplicationID:                         applicationSecond.ApplicationID,
				DeploymentName:                        "test-deployment",
				DeploymentNamespace:                   "test-namespace",
				NamespaceUID:                          "demo-namespace",
			}

			err = dbq.CreateDeploymentToApplicationMapping(ctx, deploymentToApplicationMappingsecond)
			Expect(err).To(BeNil())

			rowsAffected, err := dbq.DeleteDeploymentToApplicationMappingByDeplId(ctx, deploymentToApplicationMappingsecond.Deploymenttoapplicationmapping_uid_id)
			Expect(err).To(BeNil())
			Expect(rowsAffected).Should(Equal(1))

			err = dbq.GetDeploymentToApplicationMappingByDeplId(ctx, deploymentToApplicationMappingfirst)
			Expect(err).To(BeNil())

			err = dbq.GetDeploymentToApplicationMappingByDeplId(ctx, deploymentToApplicationMappingsecond)
			Expect(true).To(Equal(db.IsResultNotFoundError(err)))

		})

		It("Should test guard row against delete for gitopsenginecluster", func() {
			err := db.SetupForTestingDBGinkgo()
			Expect(err).To(BeNil())

			ctx := context.Background()
			dbq, err := db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).To(BeNil())
			defer dbq.CloseDatabase()

			clusterCredentialsFirst := db.ClusterCredentials{
				ClustercredentialsCredID:  "test-cluster-creds-test-1",
				Host:                      "host",
				KubeConfig:                "kube-config",
				KubeConfig_context:        "kube-config-context",
				ServiceAccountBearerToken: "serviceaccount_bearer_token",
				ServiceAccountNs:          "ServiceAccountNs",
			}

			gitopsEngineClusterFirst := db.GitopsEngineCluster{
				PrimaryKeyID:         "test-fake-cluster-1",
				ClusterCredentialsID: clusterCredentialsFirst.ClustercredentialsCredID,
			}

			err = dbq.CreateClusterCredentials(ctx, &clusterCredentialsFirst)
			Expect(err).To(BeNil())

			err = dbq.CreateGitopsEngineCluster(ctx, &gitopsEngineClusterFirst)
			Expect(err).To(BeNil())

			clusterCredentialsSecond := db.ClusterCredentials{
				ClustercredentialsCredID:  "test-cluster-creds-test-2",
				Host:                      "host",
				KubeConfig:                "kube-config",
				KubeConfig_context:        "kube-config-context",
				ServiceAccountBearerToken: "serviceaccount_bearer_token",
				ServiceAccountNs:          "ServiceAccountNs",
			}

			gitopsEngineClusterSecond := db.GitopsEngineCluster{
				PrimaryKeyID:         "test-fake-cluster-2",
				ClusterCredentialsID: clusterCredentialsSecond.ClustercredentialsCredID,
			}

			err = dbq.CreateClusterCredentials(ctx, &clusterCredentialsSecond)
			Expect(err).To(BeNil())

			err = dbq.CreateGitopsEngineCluster(ctx, &gitopsEngineClusterSecond)
			Expect(err).To(BeNil())

			rowsAffected, err := dbq.DeleteGitopsEngineClusterById(ctx, gitopsEngineClusterSecond.PrimaryKeyID)
			Expect(err).To(BeNil())
			Expect(rowsAffected).Should(Equal(1))

			err = dbq.GetGitopsEngineClusterById(ctx, &gitopsEngineClusterFirst)
			Expect(err).To(BeNil())

			err = dbq.GetGitopsEngineClusterById(ctx, &gitopsEngineClusterSecond)
			Expect(true).To(Equal(db.IsResultNotFoundError(err)))
		})

		It("Should test guard row against delete for gitopsengineinstance", func() {
			err := db.SetupForTestingDBGinkgo()
			Expect(err).To(BeNil())

			ctx := context.Background()
			dbq, err := db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).To(BeNil())
			defer dbq.CloseDatabase()

			clusterCredentialsFirst := db.ClusterCredentials{
				ClustercredentialsCredID:  "test-cluster-creds-test-1",
				Host:                      "host",
				KubeConfig:                "kube-config",
				KubeConfig_context:        "kube-config-context",
				ServiceAccountBearerToken: "serviceaccount_bearer_token",
				ServiceAccountNs:          "ServiceAccountNs",
			}

			gitopsEngineClusterFirst := db.GitopsEngineCluster{
				PrimaryKeyID:         "test-fake-cluster-1",
				ClusterCredentialsID: clusterCredentialsFirst.ClustercredentialsCredID,
			}

			gitopsEngineInstanceFirst := db.GitopsEngineInstance{
				Gitopsengineinstance_id: "test-fake-engine-instance-id-1",
				NamespaceName:           "test-fake-namespace",
				NamespaceUID:            "test-fake-namespace-1",
				EngineCluster_id:        gitopsEngineClusterFirst.PrimaryKeyID,
			}
			err = dbq.CreateClusterCredentials(ctx, &clusterCredentialsFirst)
			Expect(err).To(BeNil())

			err = dbq.CreateGitopsEngineCluster(ctx, &gitopsEngineClusterFirst)
			Expect(err).To(BeNil())

			err = dbq.CreateGitopsEngineInstance(ctx, &gitopsEngineInstanceFirst)
			Expect(err).To(BeNil())

			clusterCredentialsSecond := db.ClusterCredentials{
				ClustercredentialsCredID:  "test-cluster-creds-test-2",
				Host:                      "host",
				KubeConfig:                "kube-config",
				KubeConfig_context:        "kube-config-context",
				ServiceAccountBearerToken: "serviceaccount_bearer_token",
				ServiceAccountNs:          "ServiceAccountNs",
			}

			gitopsEngineClusterSecond := db.GitopsEngineCluster{
				PrimaryKeyID:         "test-fake-cluster-2",
				ClusterCredentialsID: clusterCredentialsSecond.ClustercredentialsCredID,
			}

			gitopsEngineInstanceSecond := db.GitopsEngineInstance{
				Gitopsengineinstance_id: "test-fake-engine-instance-id-2",
				NamespaceName:           "test-fake-namespace",
				NamespaceUID:            "test-fake-namespace-1",
				EngineCluster_id:        gitopsEngineClusterSecond.PrimaryKeyID,
			}
			err = dbq.CreateClusterCredentials(ctx, &clusterCredentialsSecond)
			Expect(err).To(BeNil())

			err = dbq.CreateGitopsEngineCluster(ctx, &gitopsEngineClusterSecond)
			Expect(err).To(BeNil())

			err = dbq.CreateGitopsEngineInstance(ctx, &gitopsEngineInstanceSecond)
			Expect(err).To(BeNil())

			rowsAffected, err := dbq.DeleteGitopsEngineInstanceById(ctx, gitopsEngineInstanceSecond.Gitopsengineinstance_id)
			Expect(err).To(BeNil())
			Expect(rowsAffected).Should(Equal(1))

			err = dbq.GetGitopsEngineInstanceById(ctx, &gitopsEngineInstanceFirst)
			Expect(err).To(BeNil())

			err = dbq.GetGitopsEngineInstanceById(ctx, &gitopsEngineInstanceSecond)
			Expect(true).To(Equal(db.IsResultNotFoundError(err)))
		})

		It("Should test guard row against delete on k8stodbmapping", func() {
			err := db.SetupForTestingDBGinkgo()
			Expect(err).To(BeNil())

			ctx := context.Background()
			dbq, err := db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).To(BeNil())
			defer dbq.CloseDatabase()

			kubernetesToDBResourceMappingFirst := db.KubernetesToDBResourceMapping{
				KubernetesResourceType: "test-resource_1",
				KubernetesResourceUID:  "test-resource_uid",
				DBRelationType:         "test-relation_type",
				DBRelationKey:          "test-relation_key",
			}
			err = dbq.CreateKubernetesResourceToDBResourceMapping(ctx, &kubernetesToDBResourceMappingFirst)
			Expect(err).To(BeNil())

			kubernetesToDBResourceMappingSecond := db.KubernetesToDBResourceMapping{
				KubernetesResourceType: "test-resource_2",
				KubernetesResourceUID:  "test-resource_uid",
				DBRelationType:         "test-relation_type",
				DBRelationKey:          "test-relation_key",
			}
			err = dbq.CreateKubernetesResourceToDBResourceMapping(ctx, &kubernetesToDBResourceMappingSecond)
			Expect(err).To(BeNil())

			rowsAffected, err := dbq.DeleteKubernetesResourceToDBResourceMapping(ctx, &kubernetesToDBResourceMappingSecond)
			Expect(err).To(BeNil())
			Expect(rowsAffected).Should(Equal(1))

			err = dbq.GetDBResourceMappingForKubernetesResource(ctx, &kubernetesToDBResourceMappingFirst)
			Expect(err).To(BeNil())

			err = dbq.GetDBResourceMappingForKubernetesResource(ctx, &kubernetesToDBResourceMappingSecond)
			Expect(true).To(Equal(db.IsResultNotFoundError(err)))

		})

		It("Should test guard row against update and delete on managedenvironment", func() {
			err := db.SetupForTestingDBGinkgo()
			Expect(err).To(BeNil())

			ctx := context.Background()
			dbq, err := db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).To(BeNil())
			defer dbq.CloseDatabase()

			clusterCredentialsFirst := db.ClusterCredentials{
				ClustercredentialsCredID:  "test-cluster-creds-test-1",
				Host:                      "host",
				KubeConfig:                "kube-config",
				KubeConfig_context:        "kube-config-context",
				ServiceAccountBearerToken: "serviceaccount_bearer_token",
				ServiceAccountNs:          "ServiceAccountNs",
			}

			managedEnvironmentFirst := db.ManagedEnvironment{
				Managedenvironment_id: "test-managed-env-1",
				ClusterCredentialsID:  clusterCredentialsFirst.ClustercredentialsCredID,
				Name:                  "my env101",
			}

			err = dbq.CreateClusterCredentials(ctx, &clusterCredentialsFirst)
			Expect(err).To(BeNil())

			err = dbq.CreateManagedEnvironment(ctx, &managedEnvironmentFirst)
			Expect(err).To(BeNil())

			clusterCredentialsSecond := db.ClusterCredentials{
				ClustercredentialsCredID:  "test-cluster-creds-test-2",
				Host:                      "host",
				KubeConfig:                "kube-config",
				KubeConfig_context:        "kube-config-context",
				ServiceAccountBearerToken: "serviceaccount_bearer_token",
				ServiceAccountNs:          "ServiceAccountNs",
			}

			managedEnvironmentSecond := db.ManagedEnvironment{
				Managedenvironment_id: "test-managed-env-2",
				ClusterCredentialsID:  clusterCredentialsSecond.ClustercredentialsCredID,
				Name:                  "my env101",
			}

			err = dbq.CreateClusterCredentials(ctx, &clusterCredentialsSecond)
			Expect(err).To(BeNil())

			err = dbq.CreateManagedEnvironment(ctx, &managedEnvironmentSecond)
			Expect(err).To(BeNil())

			managedEnvironmentSecond = db.ManagedEnvironment{
				Managedenvironment_id: "test-managed-env-2",
				ClusterCredentialsID:  clusterCredentialsSecond.ClustercredentialsCredID,
				SeqID:                 managedEnvironmentSecond.SeqID,
				Name:                  "my-env101-update",
				CreatedOn:             managedEnvironmentFirst.CreatedOn,
			}

			err = dbq.UpdateManagedEnvironment(ctx, &managedEnvironmentSecond)
			Expect(err).To(BeNil())
			err = dbq.GetManagedEnvironmentById(ctx, &managedEnvironmentFirst)
			Expect(err).To(BeNil())
			err = dbq.GetManagedEnvironmentById(ctx, &managedEnvironmentSecond)
			Expect(err).To(BeNil())

			Expect(managedEnvironmentSecond.Name).Should(Equal("my-env101-update"))
			Expect(managedEnvironmentFirst.Name).ShouldNot(Equal(managedEnvironmentSecond.Name))

			rowsAffected, err := dbq.DeleteManagedEnvironmentById(ctx, managedEnvironmentSecond.Managedenvironment_id)
			Expect(err).To(BeNil())
			Expect(rowsAffected).Should(Equal(1))

			err = dbq.GetManagedEnvironmentById(ctx, &managedEnvironmentFirst)
			Expect(err).To(BeNil())

			err = dbq.GetManagedEnvironmentById(ctx, &managedEnvironmentSecond)
			Expect(true).To(Equal(db.IsResultNotFoundError(err)))

		})

		It("Should test guard row against update and delete for operation", func() {
			err := db.SetupForTestingDBGinkgo()
			Expect(err).To(BeNil())

			ctx := context.Background()
			dbq, err := db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).To(BeNil())
			defer dbq.CloseDatabase()

			_, _, _, gitopsEngineInstance, _, err := db.CreateSampleData(dbq)
			Expect(err).To(BeNil())
			var testClusterUser = &db.ClusterUser{
				ClusterUserID: "test-user-1",
				UserName:      "test-user-1",
			}
			operationFirst := db.Operation{
				Operation_id:         "test-operation-1",
				InstanceID:           gitopsEngineInstance.Gitopsengineinstance_id,
				ResourceID:           "test-fake-resource-id",
				Resource_type:        "GitopsEngineInstance",
				State:                db.OperationState_Waiting,
				OperationOwnerUserID: testClusterUser.ClusterUserID,
			}
			err = dbq.CreateClusterUser(ctx, testClusterUser)
			Expect(err).To(BeNil())

			err = dbq.CreateOperation(ctx, &operationFirst, operationFirst.OperationOwnerUserID)
			Expect(err).To(BeNil())

			testClusterUser = &db.ClusterUser{
				ClusterUserID: "test-user-2",
				UserName:      "test-user-2",
			}
			operationSecond := db.Operation{
				Operation_id:         "test-operation-2",
				InstanceID:           gitopsEngineInstance.Gitopsengineinstance_id,
				ResourceID:           "test-fake-resource-id",
				Resource_type:        "GitopsEngineInstance",
				State:                db.OperationState_Waiting,
				OperationOwnerUserID: testClusterUser.ClusterUserID,
			}
			err = dbq.CreateClusterUser(ctx, testClusterUser)
			Expect(err).To(BeNil())

			err = dbq.CreateOperation(ctx, &operationSecond, operationSecond.OperationOwnerUserID)
			Expect(err).To(BeNil())

			operationSecond = db.Operation{
				Operation_id:         "test-operation-2",
				InstanceID:           gitopsEngineInstance.Gitopsengineinstance_id,
				ResourceID:           "test-fake-resource-id-update",
				Resource_type:        "GitopsEngineInstance",
				State:                db.OperationState_Waiting,
				OperationOwnerUserID: testClusterUser.ClusterUserID,
				SeqID:                operationSecond.SeqID,
				CreatedOn:            operationSecond.CreatedOn,
				LastStateUpdate:      operationSecond.LastStateUpdate,
			}

			err = dbq.UpdateOperation(ctx, &operationSecond)
			Expect(err).To(BeNil())
			err = dbq.GetOperationById(ctx, &operationFirst)
			Expect(err).To(BeNil())
			err = dbq.GetOperationById(ctx, &operationSecond)
			Expect(err).To(BeNil())

			Expect(operationSecond.ResourceID).Should(Equal("test-fake-resource-id-update"))
			Expect(operationFirst.ResourceID).ShouldNot(Equal(operationSecond.ResourceID))

			rowsAffected, err := dbq.DeleteOperationById(ctx, operationSecond.Operation_id)
			Expect(err).To(BeNil())
			Expect(rowsAffected).Should(Equal(1))

			err = dbq.GetOperationById(ctx, &operationFirst)
			Expect(err).To(BeNil())

			err = dbq.GetOperationById(ctx, &operationSecond)
			Expect(true).To(Equal(db.IsResultNotFoundError(err)))

		})

		It("Should test guard row against delete for syncoperation", func() {
			err := db.SetupForTestingDBGinkgo()
			Expect(err).To(BeNil())

			ctx := context.Background()
			dbq, err := db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).To(BeNil())
			defer dbq.CloseDatabase()

			_, managedEnvironment, _, gitopsEngineInstance, _, err := db.CreateSampleData(dbq)
			Expect(err).To(BeNil())

			var testClusterUser = &db.ClusterUser{
				ClusterUserID: "test-user-1",
				UserName:      "test-user-1",
			}
			err = dbq.CreateClusterUser(ctx, testClusterUser)
			Expect(err).To(BeNil())
			applicationFirst := db.Application{
				ApplicationID:          "test-my-application-1",
				Name:                   "my-application",
				SpecField:              "{}",
				EngineInstanceInstID:   gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id: managedEnvironment.Managedenvironment_id,
			}

			err = dbq.CreateApplication(ctx, &applicationFirst)
			Expect(err).To(BeNil())

			operationFirst := &db.Operation{
				Operation_id:         "test-operation-1",
				InstanceID:           gitopsEngineInstance.Gitopsengineinstance_id,
				ResourceID:           "fake resource id",
				Resource_type:        "GitopsEngineInstance",
				State:                db.OperationState_Waiting,
				OperationOwnerUserID: testClusterUser.ClusterUserID,
			}

			err = dbq.CreateOperation(ctx, operationFirst, operationFirst.OperationOwnerUserID)
			Expect(err).To(BeNil())

			syncoperationFirst := db.SyncOperation{
				SyncOperationID:     "test-sync-1",
				ApplicationID:       applicationFirst.ApplicationID,
				DeploymentNameField: "testDeployment",
				Revision:            "testRev",
				DesiredState:        "Terminated",
			}

			err = dbq.CreateSyncOperation(ctx, &syncoperationFirst)
			Expect(err).To(BeNil())

			testClusterUser = &db.ClusterUser{
				ClusterUserID: "test-user-2",
				UserName:      "test-user-2",
			}
			err = dbq.CreateClusterUser(ctx, testClusterUser)
			Expect(err).To(BeNil())

			applicationSecond := db.Application{
				ApplicationID:          "test-my-application-2",
				Name:                   "my-application",
				SpecField:              "{}",
				EngineInstanceInstID:   gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id: managedEnvironment.Managedenvironment_id,
			}

			err = dbq.CreateApplication(ctx, &applicationSecond)
			Expect(err).To(BeNil())

			operationSecond := &db.Operation{
				Operation_id:         "test-operation-2",
				InstanceID:           gitopsEngineInstance.Gitopsengineinstance_id,
				ResourceID:           "fake resource id",
				Resource_type:        "GitopsEngineInstance",
				State:                db.OperationState_Waiting,
				OperationOwnerUserID: testClusterUser.ClusterUserID,
			}

			err = dbq.CreateOperation(ctx, operationSecond, operationSecond.OperationOwnerUserID)
			Expect(err).To(BeNil())

			syncoperationSecond := db.SyncOperation{
				SyncOperationID:     "test-sync-2",
				ApplicationID:       applicationFirst.ApplicationID,
				DeploymentNameField: "testDeployment",
				Revision:            "testRev",
				DesiredState:        "Terminated",
			}

			err = dbq.CreateSyncOperation(ctx, &syncoperationSecond)
			Expect(err).To(BeNil())

			rowsAffected, err := dbq.DeleteSyncOperationById(ctx, syncoperationSecond.SyncOperationID)
			Expect(err).To(BeNil())
			Expect(rowsAffected).Should(Equal(1))

			err = dbq.GetSyncOperationById(ctx, &syncoperationFirst)
			Expect(err).To(BeNil())

			err = dbq.GetSyncOperationById(ctx, &syncoperationSecond)
			Expect(true).To(Equal(db.IsResultNotFoundError(err)))

		})

		It("Should test guard row against update and delete for repo creds", func() {
			err := db.SetupForTestingDBGinkgo()
			Expect(err).To(BeNil())

			ctx := context.Background()
			dbq, err := db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).To(BeNil())
			defer dbq.CloseDatabase()

			_, _, _, gitopsEngineInstance, _, err := db.CreateSampleData(dbq)
			Expect(err).To(BeNil())

			var testClusterUser = &db.ClusterUser{
				ClusterUserID: "test-user-1",
				UserName:      "test-user-1",
			}
			err = dbq.CreateClusterUser(ctx, testClusterUser)
			Expect(err).To(BeNil())

			gitopsRepositoryCredentialsFirst := db.RepositoryCredentials{
				RepositoryCredentialsID: "test-repo-cred-id",
				UserID:                  testClusterUser.ClusterUserID, // constrain 'fk_clusteruser_id'
				PrivateURL:              "https://test-private-url",
				AuthUsername:            "test-auth-username",
				AuthPassword:            "test-auth-password",
				AuthSSHKey:              "test-auth-ssh-key",
				SecretObj:               "test-secret-obj",
				EngineClusterID:         gitopsEngineInstance.Gitopsengineinstance_id, // constrain 'fk_gitopsengineinstance_id'
				CreatedOn:               time.Now(),
			}
			err = dbq.CreateRepositoryCredentials(ctx, &gitopsRepositoryCredentialsFirst)
			Expect(err).To(BeNil())

			testClusterUser = &db.ClusterUser{
				ClusterUserID: "test-user-2",
				UserName:      "test-user-2",
			}
			err = dbq.CreateClusterUser(ctx, testClusterUser)
			Expect(err).To(BeNil())

			gitopsRepositoryCredentialsSecond := db.RepositoryCredentials{
				RepositoryCredentialsID: "test-repo-cred-id-2",
				UserID:                  testClusterUser.ClusterUserID, // constrain 'fk_clusteruser_id'
				PrivateURL:              "https://test-private-url-2",
				AuthUsername:            "test-auth-username-2",
				AuthPassword:            "test-auth-password-2",
				AuthSSHKey:              "test-auth-ssh-key-2",
				SecretObj:               "test-secret-obj-2",
				EngineClusterID:         gitopsEngineInstance.Gitopsengineinstance_id, // constrain 'fk_gitopsengineinstance_id'
				CreatedOn:               time.Now(),
			}
			err = dbq.CreateRepositoryCredentials(ctx, &gitopsRepositoryCredentialsSecond)
			Expect(err).To(BeNil())

			fetch, err := dbq.GetRepositoryCredentialsByID(ctx, gitopsRepositoryCredentialsSecond.RepositoryCredentialsID)
			Expect(err).To(BeNil())

			gitopsRepositoryCredentialsSecond = fetch
			gitopsRepositoryCredentialsSecond.AuthUsername = "updated-auth-username"

			err = dbq.UpdateRepositoryCredentials(ctx, &gitopsRepositoryCredentialsSecond)
			Expect(err).To(BeNil())

			gitopsRepositoryCredentialsFirst, err = dbq.GetRepositoryCredentialsByID(ctx, gitopsRepositoryCredentialsFirst.RepositoryCredentialsID)
			Expect(err).To(BeNil())
			gitopsRepositoryCredentialsSecond, err = dbq.GetRepositoryCredentialsByID(ctx, gitopsRepositoryCredentialsSecond.RepositoryCredentialsID)
			Expect(err).To(BeNil())

			Expect(gitopsRepositoryCredentialsSecond.AuthUsername).Should(Equal("updated-auth-username"))
			Expect(gitopsRepositoryCredentialsFirst.AuthUsername).ShouldNot(Equal(gitopsRepositoryCredentialsSecond.AuthUsername))

			rowsAffected, err := dbq.DeleteRepositoryCredentialsByID(ctx, gitopsRepositoryCredentialsSecond.RepositoryCredentialsID)
			Expect(err).To(BeNil())
			Expect(rowsAffected).Should(Equal(1))

			_, err = dbq.GetRepositoryCredentialsByID(ctx, gitopsRepositoryCredentialsFirst.RepositoryCredentialsID)
			Expect(err).To(BeNil())

			_, err = dbq.GetRepositoryCredentialsByID(ctx, gitopsRepositoryCredentialsSecond.RepositoryCredentialsID)
			Expect(true).To(Equal(db.IsResultNotFoundError(err)))

		})
	})
})
