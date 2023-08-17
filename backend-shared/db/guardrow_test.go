package db_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/db"
)

var _ = Describe("Test to verify update/delete operations are not globally scoped", func() {
	Context("It creates database unit tests which guard against missing WHERE clauses of UPDATE/DELETE operations to the database ", func() {
		var err error
		var dbq db.AllDatabaseQueries
		var ctx context.Context

		BeforeEach(func() {
			err = db.SetupForTestingDBGinkgo()
			Expect(err).ToNot(HaveOccurred())

			ctx = context.Background()
			dbq, err = db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).ToNot(HaveOccurred())
		})

		AfterEach(func() {
			dbq.CloseDatabase()
		})

		It("Should test guard row against delete for ApiCRtoDBmapping", func() {
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
			Expect(err).ToNot(HaveOccurred())

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
			Expect(err).ToNot(HaveOccurred())

			rowsAffected, err := dbq.DeleteAPICRToDatabaseMapping(ctx, &ApicrtodatabasemappingSecond)
			Expect(err).ToNot(HaveOccurred())
			Expect(rowsAffected).To(Equal((1)))

			err = dbq.GetDatabaseMappingForAPICR(ctx, &ApicrtodatabasemappingFirst)
			Expect(err).ToNot(HaveOccurred())

			err = dbq.GetDatabaseMappingForAPICR(ctx, &ApicrtodatabasemappingSecond)
			Expect(true).To(Equal(db.IsResultNotFoundError(err)))

		})

		It("Should test guard row against update and delete on application", func() {
			_, managedEnvironment, _, gitopsEngineInstance, _, err := db.CreateSampleData(dbq)
			Expect(err).ToNot(HaveOccurred())

			applicationFirst := db.Application{
				Application_id:          "test-my-application-1",
				Name:                    "my-application",
				Spec_field:              "{}",
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironment.Managedenvironment_id,
			}

			err = dbq.CreateApplication(ctx, &applicationFirst)
			Expect(err).ToNot(HaveOccurred())

			applicationSecond := db.Application{
				Application_id:          "test-my-application-2",
				Name:                    "my-application",
				Spec_field:              "{}",
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironment.Managedenvironment_id,
			}
			err = dbq.CreateApplication(ctx, &applicationSecond)
			Expect(err).ToNot(HaveOccurred())

			applicationSecond = db.Application{
				Application_id:          applicationSecond.Application_id,
				Name:                    "test-application-update",
				Spec_field:              "{}",
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironment.Managedenvironment_id,
				SeqID:                   applicationSecond.SeqID,
				Created_on:              applicationFirst.Created_on,
			}

			err = dbq.UpdateApplication(ctx, &applicationSecond)
			Expect(err).ToNot(HaveOccurred())
			err = dbq.GetApplicationById(ctx, &applicationFirst)
			Expect(err).ToNot(HaveOccurred())
			err = dbq.GetApplicationById(ctx, &applicationSecond)
			Expect(err).ToNot(HaveOccurred())
			Expect(applicationSecond.Name).Should(Equal("test-application-update"))
			Expect(applicationFirst.Name).ShouldNot(Equal(applicationSecond.Name))

			rowsAffected, err := dbq.DeleteApplicationById(ctx, applicationSecond.Application_id)
			Expect(err).ToNot(HaveOccurred())
			Expect(rowsAffected).Should(Equal(1))

			err = dbq.GetApplicationById(ctx, &applicationFirst)
			Expect(err).ToNot(HaveOccurred())

			err = dbq.GetApplicationById(ctx, &applicationSecond)
			Expect(true).To(Equal(db.IsResultNotFoundError(err)))

		})

		It("Should test guard row against update and delete for applicationstates", func() {
			_, managedEnvironment, _, gitopsEngineInstance, _, err := db.CreateSampleData(dbq)
			Expect(err).ToNot(HaveOccurred())

			applicationFirst := db.Application{
				Application_id:          "test-my-application-1",
				Name:                    "my-application",
				Spec_field:              "{}",
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironment.Managedenvironment_id,
			}

			err = dbq.CreateApplication(ctx, &applicationFirst)
			Expect(err).ToNot(HaveOccurred())

			applicationStateFirst := &db.ApplicationState{
				Applicationstate_application_id: applicationFirst.Application_id,
				Health:                          "Progressing",
				Sync_Status:                     "Unknown",
				Resources:                       make([]byte, 10),
				ReconciledState:                 "test-reconciledState",
				Conditions:                      []byte("sample"),
			}

			err = dbq.CreateApplicationState(ctx, applicationStateFirst)
			Expect(err).ToNot(HaveOccurred())

			applicationSecond := db.Application{
				Application_id:          "test-my-application-2",
				Name:                    "my-application",
				Spec_field:              "{}",
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironment.Managedenvironment_id,
			}
			err = dbq.CreateApplication(ctx, &applicationSecond)
			Expect(err).ToNot(HaveOccurred())

			applicationStateSecond := &db.ApplicationState{
				Applicationstate_application_id: applicationSecond.Application_id,
				Health:                          "Progressing",
				Sync_Status:                     "Unknown",
				Resources:                       make([]byte, 10),
				Conditions:                      []byte("sample"),
				ReconciledState:                 "sample",
			}

			err = dbq.CreateApplicationState(ctx, applicationStateSecond)
			Expect(err).ToNot(HaveOccurred())
			applicationStateSecond = &db.ApplicationState{
				Applicationstate_application_id: applicationSecond.Application_id,
				Health:                          "Progressing",
				Sync_Status:                     "Sync",
				Resources:                       make([]byte, 10),
				Conditions:                      []byte("sample"),
				ReconciledState:                 "sample",
			}

			err = dbq.UpdateApplicationState(ctx, applicationStateSecond)
			Expect(err).ToNot(HaveOccurred())
			err = dbq.GetApplicationStateById(ctx, applicationStateFirst)
			Expect(err).ToNot(HaveOccurred())
			err = dbq.GetApplicationStateById(ctx, applicationStateSecond)
			Expect(err).ToNot(HaveOccurred())

			Expect(applicationStateSecond.Sync_Status).Should(Equal("Sync"))
			Expect(applicationStateFirst.Sync_Status).ShouldNot(Equal(applicationStateSecond.Sync_Status))

			rowsAffected, err := dbq.DeleteApplicationStateById(ctx, applicationStateSecond.Applicationstate_application_id)
			Expect(err).ToNot(HaveOccurred())
			Expect(rowsAffected).Should(Equal(1))

			err = dbq.GetApplicationStateById(ctx, applicationStateFirst)
			Expect(err).ToNot(HaveOccurred())

			err = dbq.GetApplicationStateById(ctx, applicationStateSecond)
			Expect(true).To(Equal(db.IsResultNotFoundError(err)))

		})

		It("Should test guard row against delete for clusteraccess", func() {
			var clusterUser = &db.ClusterUser{
				Clusteruser_id: "test-user-1",
				User_name:      "test-user-1",
			}
			err = dbq.CreateClusterUser(ctx, clusterUser)
			Expect(err).ToNot(HaveOccurred())

			clusterCredentialsFirst := db.ClusterCredentials{
				Clustercredentials_cred_id:  "test-cluster-creds-test-1",
				Host:                        "host",
				Kube_config:                 "kube-config",
				Kube_config_context:         "kube-config-context",
				Serviceaccount_bearer_token: "serviceaccount_bearer_token",
				Serviceaccount_ns:           "Serviceaccount_ns",
			}

			managedEnvironmentFirst := db.ManagedEnvironment{
				Managedenvironment_id: "test-managed-env-1",
				Clustercredentials_id: clusterCredentialsFirst.Clustercredentials_cred_id,
				Name:                  "my env",
			}

			gitopsEngineClusterFirst := db.GitopsEngineCluster{
				Gitopsenginecluster_id: "test-fake-cluster-1",
				Clustercredentials_id:  clusterCredentialsFirst.Clustercredentials_cred_id,
			}

			gitopsEngineInstanceFirst := db.GitopsEngineInstance{
				Gitopsengineinstance_id: "test-fake-engine-instance-id-1",
				Namespace_name:          "test-fake-namespace",
				Namespace_uid:           "test-fake-namespace-5",
				EngineCluster_id:        gitopsEngineClusterFirst.Gitopsenginecluster_id,
			}

			clusterAccessFirst := db.ClusterAccess{
				Clusteraccess_user_id:                   clusterUser.Clusteruser_id,
				Clusteraccess_managed_environment_id:    managedEnvironmentFirst.Managedenvironment_id,
				Clusteraccess_gitops_engine_instance_id: gitopsEngineInstanceFirst.Gitopsengineinstance_id,
			}

			err = dbq.CreateClusterCredentials(ctx, &clusterCredentialsFirst)
			Expect(err).ToNot(HaveOccurred())

			err = dbq.CreateManagedEnvironment(ctx, &managedEnvironmentFirst)
			Expect(err).ToNot(HaveOccurred())

			err = dbq.CreateGitopsEngineCluster(ctx, &gitopsEngineClusterFirst)
			Expect(err).ToNot(HaveOccurred())

			err = dbq.CreateGitopsEngineInstance(ctx, &gitopsEngineInstanceFirst)
			Expect(err).ToNot(HaveOccurred())

			err = dbq.CreateClusterAccess(ctx, &clusterAccessFirst)
			Expect(err).ToNot(HaveOccurred())

			clusterUser = &db.ClusterUser{
				Clusteruser_id: "test-user-2",
				User_name:      "test-user-2",
			}
			err = dbq.CreateClusterUser(ctx, clusterUser)
			Expect(err).ToNot(HaveOccurred())

			clusterCredentialsSecond := db.ClusterCredentials{
				Clustercredentials_cred_id:  "test-cluster-creds-test-2",
				Host:                        "host",
				Kube_config:                 "kube-config",
				Kube_config_context:         "kube-config-context",
				Serviceaccount_bearer_token: "serviceaccount_bearer_token",
				Serviceaccount_ns:           "Serviceaccount_ns",
			}

			managedEnvironmentSecond := db.ManagedEnvironment{
				Managedenvironment_id: "test-managed-env-2",
				Clustercredentials_id: clusterCredentialsSecond.Clustercredentials_cred_id,
				Name:                  "my env",
			}

			gitopsEngineClusterSecond := db.GitopsEngineCluster{
				Gitopsenginecluster_id: "test-fake-cluster-2",
				Clustercredentials_id:  clusterCredentialsSecond.Clustercredentials_cred_id,
			}

			gitopsEngineInstanceSecond := db.GitopsEngineInstance{
				Gitopsengineinstance_id: "test-fake-engine-instance-id-2",
				Namespace_name:          "test-fake-namespace",
				Namespace_uid:           "test-fake-namespace-5",
				EngineCluster_id:        gitopsEngineClusterSecond.Gitopsenginecluster_id,
			}

			clusterAccessSecond := db.ClusterAccess{
				Clusteraccess_user_id:                   clusterUser.Clusteruser_id,
				Clusteraccess_managed_environment_id:    managedEnvironmentSecond.Managedenvironment_id,
				Clusteraccess_gitops_engine_instance_id: gitopsEngineInstanceSecond.Gitopsengineinstance_id,
			}

			err = dbq.CreateClusterCredentials(ctx, &clusterCredentialsSecond)
			Expect(err).ToNot(HaveOccurred())

			err = dbq.CreateManagedEnvironment(ctx, &managedEnvironmentSecond)
			Expect(err).ToNot(HaveOccurred())

			err = dbq.CreateGitopsEngineCluster(ctx, &gitopsEngineClusterSecond)
			Expect(err).ToNot(HaveOccurred())

			err = dbq.CreateGitopsEngineInstance(ctx, &gitopsEngineInstanceSecond)
			Expect(err).ToNot(HaveOccurred())

			err = dbq.CreateClusterAccess(ctx, &clusterAccessSecond)
			Expect(err).ToNot(HaveOccurred())

			affectedRows, err := dbq.DeleteClusterAccessById(ctx, clusterAccessSecond.Clusteraccess_user_id, clusterAccessSecond.Clusteraccess_managed_environment_id, clusterAccessSecond.Clusteraccess_gitops_engine_instance_id)
			Expect(err).ToNot(HaveOccurred())
			Expect(affectedRows).To(Equal(1))

			err = dbq.GetClusterAccessByPrimaryKey(ctx, &clusterAccessFirst)
			Expect(err).ToNot(HaveOccurred())

			err = dbq.GetClusterAccessByPrimaryKey(ctx, &clusterAccessSecond)
			Expect(true).To(Equal(db.IsResultNotFoundError(err)))

		})

		It("Should test guard row against delete for clustercredentials", func() {
			clusterCredFirst := db.ClusterCredentials{
				Host:                        "test-host",
				Kube_config:                 "test-kube_config",
				Kube_config_context:         "test-kube_config_context",
				Serviceaccount_bearer_token: "test-serviceaccount_bearer_token",
				Serviceaccount_ns:           "test-serviceaccount_ns",
			}
			err = dbq.CreateClusterCredentials(ctx, &clusterCredFirst)
			Expect(err).ToNot(HaveOccurred())

			clusterCredSecond := db.ClusterCredentials{
				Host:                        "test-host",
				Kube_config:                 "test-kube_config",
				Kube_config_context:         "test-kube_config_context",
				Serviceaccount_bearer_token: "test-serviceaccount_bearer_token",
				Serviceaccount_ns:           "test-serviceaccount_ns",
			}
			err = dbq.CreateClusterCredentials(ctx, &clusterCredSecond)
			Expect(err).ToNot(HaveOccurred())

			rowsAffected, err := dbq.DeleteClusterCredentialsById(ctx, clusterCredSecond.Clustercredentials_cred_id)
			Expect(err).ToNot(HaveOccurred())
			Expect(rowsAffected).Should(Equal(1))

			err = dbq.GetClusterCredentialsById(ctx, &clusterCredFirst)
			Expect(err).ToNot(HaveOccurred())

			err = dbq.GetClusterCredentialsById(ctx, &clusterCredSecond)
			Expect(true).To(Equal(db.IsResultNotFoundError(err)))

		})

		It("Should test guard row against delete for clusteruser", func() {
			userfirst := &db.ClusterUser{
				Clusteruser_id: "test-user-id-1",
				User_name:      "test-user-1",
			}
			err = dbq.CreateClusterUser(ctx, userfirst)
			Expect(err).ToNot(HaveOccurred())

			usersecond := &db.ClusterUser{
				Clusteruser_id: "test-user-id-2",
				User_name:      "test-user-2",
			}
			err = dbq.CreateClusterUser(ctx, usersecond)
			Expect(err).ToNot(HaveOccurred())

			rowsAffected, err := dbq.DeleteClusterUserById(ctx, usersecond.Clusteruser_id)
			Expect(err).ToNot(HaveOccurred())
			Expect(rowsAffected).Should(Equal(1))

			err = dbq.GetClusterUserById(ctx, userfirst)
			Expect(err).ToNot(HaveOccurred())

			err = dbq.GetClusterUserById(ctx, usersecond)
			Expect(true).To(Equal(db.IsResultNotFoundError(err)))

		})

		It("Should test guard row against delete for deploymenttoapplicationmapping", func() {
			_, managedEnvironment, _, gitopsEngineInstance, _, err := db.CreateSampleData(dbq)
			Expect(err).ToNot(HaveOccurred())

			applicationFirst := db.Application{
				Application_id:          "test-my-application-1",
				Name:                    "my-application",
				Spec_field:              "{}",
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironment.Managedenvironment_id,
			}

			err = dbq.CreateApplication(ctx, &applicationFirst)
			Expect(err).ToNot(HaveOccurred())

			deploymentToApplicationMappingfirst := &db.DeploymentToApplicationMapping{
				Deploymenttoapplicationmapping_uid_id: "test-" + generateUuid(),
				Application_id:                        applicationFirst.Application_id,
				DeploymentName:                        "test-deployment",
				DeploymentNamespace:                   "test-namespace",
				NamespaceUID:                          "demo-namespace",
			}

			err = dbq.CreateDeploymentToApplicationMapping(ctx, deploymentToApplicationMappingfirst)
			Expect(err).ToNot(HaveOccurred())

			applicationSecond := db.Application{
				Application_id:          "test-my-application-2",
				Name:                    "my-application",
				Spec_field:              "{}",
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironment.Managedenvironment_id,
			}
			err = dbq.CreateApplication(ctx, &applicationSecond)
			Expect(err).ToNot(HaveOccurred())

			deploymentToApplicationMappingsecond := &db.DeploymentToApplicationMapping{
				Deploymenttoapplicationmapping_uid_id: "test-" + generateUuid(),
				Application_id:                        applicationSecond.Application_id,
				DeploymentName:                        "test-deployment",
				DeploymentNamespace:                   "test-namespace",
				NamespaceUID:                          "demo-namespace",
			}

			err = dbq.CreateDeploymentToApplicationMapping(ctx, deploymentToApplicationMappingsecond)
			Expect(err).ToNot(HaveOccurred())

			rowsAffected, err := dbq.DeleteDeploymentToApplicationMappingByDeplId(ctx, deploymentToApplicationMappingsecond.Deploymenttoapplicationmapping_uid_id)
			Expect(err).ToNot(HaveOccurred())
			Expect(rowsAffected).Should(Equal(1))

			err = dbq.GetDeploymentToApplicationMappingByDeplId(ctx, deploymentToApplicationMappingfirst)
			Expect(err).ToNot(HaveOccurred())

			err = dbq.GetDeploymentToApplicationMappingByDeplId(ctx, deploymentToApplicationMappingsecond)
			Expect(true).To(Equal(db.IsResultNotFoundError(err)))

		})

		It("Should test guard row against delete for gitopsenginecluster", func() {
			clusterCredentialsFirst := db.ClusterCredentials{
				Clustercredentials_cred_id:  "test-cluster-creds-test-1",
				Host:                        "host",
				Kube_config:                 "kube-config",
				Kube_config_context:         "kube-config-context",
				Serviceaccount_bearer_token: "serviceaccount_bearer_token",
				Serviceaccount_ns:           "Serviceaccount_ns",
			}

			gitopsEngineClusterFirst := db.GitopsEngineCluster{
				Gitopsenginecluster_id: "test-fake-cluster-1",
				Clustercredentials_id:  clusterCredentialsFirst.Clustercredentials_cred_id,
			}

			err = dbq.CreateClusterCredentials(ctx, &clusterCredentialsFirst)
			Expect(err).ToNot(HaveOccurred())

			err = dbq.CreateGitopsEngineCluster(ctx, &gitopsEngineClusterFirst)
			Expect(err).ToNot(HaveOccurred())

			clusterCredentialsSecond := db.ClusterCredentials{
				Clustercredentials_cred_id:  "test-cluster-creds-test-2",
				Host:                        "host",
				Kube_config:                 "kube-config",
				Kube_config_context:         "kube-config-context",
				Serviceaccount_bearer_token: "serviceaccount_bearer_token",
				Serviceaccount_ns:           "Serviceaccount_ns",
			}

			gitopsEngineClusterSecond := db.GitopsEngineCluster{
				Gitopsenginecluster_id: "test-fake-cluster-2",
				Clustercredentials_id:  clusterCredentialsSecond.Clustercredentials_cred_id,
			}

			err = dbq.CreateClusterCredentials(ctx, &clusterCredentialsSecond)
			Expect(err).ToNot(HaveOccurred())

			err = dbq.CreateGitopsEngineCluster(ctx, &gitopsEngineClusterSecond)
			Expect(err).ToNot(HaveOccurred())

			rowsAffected, err := dbq.DeleteGitopsEngineClusterById(ctx, gitopsEngineClusterSecond.Gitopsenginecluster_id)
			Expect(err).ToNot(HaveOccurred())
			Expect(rowsAffected).Should(Equal(1))

			err = dbq.GetGitopsEngineClusterById(ctx, &gitopsEngineClusterFirst)
			Expect(err).ToNot(HaveOccurred())

			err = dbq.GetGitopsEngineClusterById(ctx, &gitopsEngineClusterSecond)
			Expect(true).To(Equal(db.IsResultNotFoundError(err)))
		})

		It("Should test guard row against delete for gitopsengineinstance", func() {
			clusterCredentialsFirst := db.ClusterCredentials{
				Clustercredentials_cred_id:  "test-cluster-creds-test-1",
				Host:                        "host",
				Kube_config:                 "kube-config",
				Kube_config_context:         "kube-config-context",
				Serviceaccount_bearer_token: "serviceaccount_bearer_token",
				Serviceaccount_ns:           "Serviceaccount_ns",
			}

			gitopsEngineClusterFirst := db.GitopsEngineCluster{
				Gitopsenginecluster_id: "test-fake-cluster-1",
				Clustercredentials_id:  clusterCredentialsFirst.Clustercredentials_cred_id,
			}

			gitopsEngineInstanceFirst := db.GitopsEngineInstance{
				Gitopsengineinstance_id: "test-fake-engine-instance-id-1",
				Namespace_name:          "test-fake-namespace",
				Namespace_uid:           "test-fake-namespace-1",
				EngineCluster_id:        gitopsEngineClusterFirst.Gitopsenginecluster_id,
			}
			err = dbq.CreateClusterCredentials(ctx, &clusterCredentialsFirst)
			Expect(err).ToNot(HaveOccurred())

			err = dbq.CreateGitopsEngineCluster(ctx, &gitopsEngineClusterFirst)
			Expect(err).ToNot(HaveOccurred())

			err = dbq.CreateGitopsEngineInstance(ctx, &gitopsEngineInstanceFirst)
			Expect(err).ToNot(HaveOccurred())

			clusterCredentialsSecond := db.ClusterCredentials{
				Clustercredentials_cred_id:  "test-cluster-creds-test-2",
				Host:                        "host",
				Kube_config:                 "kube-config",
				Kube_config_context:         "kube-config-context",
				Serviceaccount_bearer_token: "serviceaccount_bearer_token",
				Serviceaccount_ns:           "Serviceaccount_ns",
			}

			gitopsEngineClusterSecond := db.GitopsEngineCluster{
				Gitopsenginecluster_id: "test-fake-cluster-2",
				Clustercredentials_id:  clusterCredentialsSecond.Clustercredentials_cred_id,
			}

			gitopsEngineInstanceSecond := db.GitopsEngineInstance{
				Gitopsengineinstance_id: "test-fake-engine-instance-id-2",
				Namespace_name:          "test-fake-namespace",
				Namespace_uid:           "test-fake-namespace-1",
				EngineCluster_id:        gitopsEngineClusterSecond.Gitopsenginecluster_id,
			}
			err = dbq.CreateClusterCredentials(ctx, &clusterCredentialsSecond)
			Expect(err).ToNot(HaveOccurred())

			err = dbq.CreateGitopsEngineCluster(ctx, &gitopsEngineClusterSecond)
			Expect(err).ToNot(HaveOccurred())

			err = dbq.CreateGitopsEngineInstance(ctx, &gitopsEngineInstanceSecond)
			Expect(err).ToNot(HaveOccurred())

			rowsAffected, err := dbq.DeleteGitopsEngineInstanceById(ctx, gitopsEngineInstanceSecond.Gitopsengineinstance_id)
			Expect(err).ToNot(HaveOccurred())
			Expect(rowsAffected).Should(Equal(1))

			err = dbq.GetGitopsEngineInstanceById(ctx, &gitopsEngineInstanceFirst)
			Expect(err).ToNot(HaveOccurred())

			err = dbq.GetGitopsEngineInstanceById(ctx, &gitopsEngineInstanceSecond)
			Expect(true).To(Equal(db.IsResultNotFoundError(err)))
		})

		It("Should test guard row against delete on k8stodbmapping", func() {
			err := db.SetupForTestingDBGinkgo()
			Expect(err).ToNot(HaveOccurred())

			ctx := context.Background()
			dbq, err := db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).ToNot(HaveOccurred())

			kubernetesToDBResourceMappingFirst := db.KubernetesToDBResourceMapping{
				KubernetesResourceType: "test-resource_1",
				KubernetesResourceUID:  "test-resource_uid",
				DBRelationType:         "test-relation_type",
				DBRelationKey:          "test-relation_key",
			}
			err = dbq.CreateKubernetesResourceToDBResourceMapping(ctx, &kubernetesToDBResourceMappingFirst)
			Expect(err).ToNot(HaveOccurred())

			kubernetesToDBResourceMappingSecond := db.KubernetesToDBResourceMapping{
				KubernetesResourceType: "test-resource_2",
				KubernetesResourceUID:  "test-resource_uid",
				DBRelationType:         "test-relation_type",
				DBRelationKey:          "test-relation_key",
			}
			err = dbq.CreateKubernetesResourceToDBResourceMapping(ctx, &kubernetesToDBResourceMappingSecond)
			Expect(err).ToNot(HaveOccurred())

			rowsAffected, err := dbq.DeleteKubernetesResourceToDBResourceMapping(ctx, &kubernetesToDBResourceMappingSecond)
			Expect(err).ToNot(HaveOccurred())
			Expect(rowsAffected).Should(Equal(1))

			err = dbq.GetDBResourceMappingForKubernetesResource(ctx, &kubernetesToDBResourceMappingFirst)
			Expect(err).ToNot(HaveOccurred())

			err = dbq.GetDBResourceMappingForKubernetesResource(ctx, &kubernetesToDBResourceMappingSecond)
			Expect(true).To(Equal(db.IsResultNotFoundError(err)))

		})

		It("Should test guard row against update and delete on managedenvironment", func() {
			clusterCredentialsFirst := db.ClusterCredentials{
				Clustercredentials_cred_id:  "test-cluster-creds-test-1",
				Host:                        "host",
				Kube_config:                 "kube-config",
				Kube_config_context:         "kube-config-context",
				Serviceaccount_bearer_token: "serviceaccount_bearer_token",
				Serviceaccount_ns:           "Serviceaccount_ns",
			}

			managedEnvironmentFirst := db.ManagedEnvironment{
				Managedenvironment_id: "test-managed-env-1",
				Clustercredentials_id: clusterCredentialsFirst.Clustercredentials_cred_id,
				Name:                  "my env101",
			}

			err = dbq.CreateClusterCredentials(ctx, &clusterCredentialsFirst)
			Expect(err).ToNot(HaveOccurred())

			err = dbq.CreateManagedEnvironment(ctx, &managedEnvironmentFirst)
			Expect(err).ToNot(HaveOccurred())

			clusterCredentialsSecond := db.ClusterCredentials{
				Clustercredentials_cred_id:  "test-cluster-creds-test-2",
				Host:                        "host",
				Kube_config:                 "kube-config",
				Kube_config_context:         "kube-config-context",
				Serviceaccount_bearer_token: "serviceaccount_bearer_token",
				Serviceaccount_ns:           "Serviceaccount_ns",
			}

			managedEnvironmentSecond := db.ManagedEnvironment{
				Managedenvironment_id: "test-managed-env-2",
				Clustercredentials_id: clusterCredentialsSecond.Clustercredentials_cred_id,
				Name:                  "my env101",
			}

			err = dbq.CreateClusterCredentials(ctx, &clusterCredentialsSecond)
			Expect(err).ToNot(HaveOccurred())

			err = dbq.CreateManagedEnvironment(ctx, &managedEnvironmentSecond)
			Expect(err).ToNot(HaveOccurred())

			managedEnvironmentSecond = db.ManagedEnvironment{
				Managedenvironment_id: "test-managed-env-2",
				Clustercredentials_id: clusterCredentialsSecond.Clustercredentials_cred_id,
				SeqID:                 managedEnvironmentSecond.SeqID,
				Name:                  "my-env101-update",
				Created_on:            managedEnvironmentFirst.Created_on,
			}

			err = dbq.UpdateManagedEnvironment(ctx, &managedEnvironmentSecond)
			Expect(err).ToNot(HaveOccurred())
			err = dbq.GetManagedEnvironmentById(ctx, &managedEnvironmentFirst)
			Expect(err).ToNot(HaveOccurred())
			err = dbq.GetManagedEnvironmentById(ctx, &managedEnvironmentSecond)
			Expect(err).ToNot(HaveOccurred())

			Expect(managedEnvironmentSecond.Name).Should(Equal("my-env101-update"))
			Expect(managedEnvironmentFirst.Name).ShouldNot(Equal(managedEnvironmentSecond.Name))

			rowsAffected, err := dbq.DeleteManagedEnvironmentById(ctx, managedEnvironmentSecond.Managedenvironment_id)
			Expect(err).ToNot(HaveOccurred())
			Expect(rowsAffected).Should(Equal(1))

			err = dbq.GetManagedEnvironmentById(ctx, &managedEnvironmentFirst)
			Expect(err).ToNot(HaveOccurred())

			err = dbq.GetManagedEnvironmentById(ctx, &managedEnvironmentSecond)
			Expect(true).To(Equal(db.IsResultNotFoundError(err)))

		})

		It("Should test guard row against update and delete for operation", func() {
			_, _, _, gitopsEngineInstance, _, err := db.CreateSampleData(dbq)
			Expect(err).ToNot(HaveOccurred())
			var testClusterUser = &db.ClusterUser{
				Clusteruser_id: "test-user-1",
				User_name:      "test-user-1",
			}
			operationFirst := db.Operation{
				Operation_id:            "test-operation-1",
				Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
				Resource_id:             "test-fake-resource-id",
				Resource_type:           "GitopsEngineInstance",
				State:                   db.OperationState_Waiting,
				Operation_owner_user_id: testClusterUser.Clusteruser_id,
			}
			err = dbq.CreateClusterUser(ctx, testClusterUser)
			Expect(err).ToNot(HaveOccurred())

			err = dbq.CreateOperation(ctx, &operationFirst, operationFirst.Operation_owner_user_id)
			Expect(err).ToNot(HaveOccurred())

			testClusterUser = &db.ClusterUser{
				Clusteruser_id: "test-user-2",
				User_name:      "test-user-2",
			}
			operationSecond := db.Operation{
				Operation_id:            "test-operation-2",
				Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
				Resource_id:             "test-fake-resource-id",
				Resource_type:           "GitopsEngineInstance",
				State:                   db.OperationState_Waiting,
				Operation_owner_user_id: testClusterUser.Clusteruser_id,
			}
			err = dbq.CreateClusterUser(ctx, testClusterUser)
			Expect(err).ToNot(HaveOccurred())

			err = dbq.CreateOperation(ctx, &operationSecond, operationSecond.Operation_owner_user_id)
			Expect(err).ToNot(HaveOccurred())

			operationSecond = db.Operation{
				Operation_id:            "test-operation-2",
				Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
				Resource_id:             "test-fake-resource-id-update",
				Resource_type:           "GitopsEngineInstance",
				State:                   db.OperationState_Waiting,
				Operation_owner_user_id: testClusterUser.Clusteruser_id,
				SeqID:                   operationSecond.SeqID,
				Created_on:              operationSecond.Created_on,
				Last_state_update:       operationSecond.Last_state_update,
			}

			err = dbq.UpdateOperation(ctx, &operationSecond)
			Expect(err).ToNot(HaveOccurred())
			err = dbq.GetOperationById(ctx, &operationFirst)
			Expect(err).ToNot(HaveOccurred())
			err = dbq.GetOperationById(ctx, &operationSecond)
			Expect(err).ToNot(HaveOccurred())

			Expect(operationSecond.Resource_id).Should(Equal("test-fake-resource-id-update"))
			Expect(operationFirst.Resource_id).ShouldNot(Equal(operationSecond.Resource_id))

			rowsAffected, err := dbq.DeleteOperationById(ctx, operationSecond.Operation_id)
			Expect(err).ToNot(HaveOccurred())
			Expect(rowsAffected).Should(Equal(1))

			err = dbq.GetOperationById(ctx, &operationFirst)
			Expect(err).ToNot(HaveOccurred())

			err = dbq.GetOperationById(ctx, &operationSecond)
			Expect(true).To(Equal(db.IsResultNotFoundError(err)))

		})

		It("Should test guard row against delete for syncoperation", func() {
			_, managedEnvironment, _, gitopsEngineInstance, _, err := db.CreateSampleData(dbq)
			Expect(err).ToNot(HaveOccurred())

			var testClusterUser = &db.ClusterUser{
				Clusteruser_id: "test-user-1",
				User_name:      "test-user-1",
			}
			err = dbq.CreateClusterUser(ctx, testClusterUser)
			Expect(err).ToNot(HaveOccurred())
			applicationFirst := db.Application{
				Application_id:          "test-my-application-1",
				Name:                    "my-application",
				Spec_field:              "{}",
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironment.Managedenvironment_id,
			}

			err = dbq.CreateApplication(ctx, &applicationFirst)
			Expect(err).ToNot(HaveOccurred())

			operationFirst := &db.Operation{
				Operation_id:            "test-operation-1",
				Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
				Resource_id:             "fake resource id",
				Resource_type:           "GitopsEngineInstance",
				State:                   db.OperationState_Waiting,
				Operation_owner_user_id: testClusterUser.Clusteruser_id,
			}

			err = dbq.CreateOperation(ctx, operationFirst, operationFirst.Operation_owner_user_id)
			Expect(err).ToNot(HaveOccurred())

			syncoperationFirst := db.SyncOperation{
				SyncOperation_id:    "test-sync-1",
				Application_id:      applicationFirst.Application_id,
				DeploymentNameField: "testDeployment",
				Revision:            "testRev",
				DesiredState:        "Terminated",
			}

			err = dbq.CreateSyncOperation(ctx, &syncoperationFirst)
			Expect(err).ToNot(HaveOccurred())

			testClusterUser = &db.ClusterUser{
				Clusteruser_id: "test-user-2",
				User_name:      "test-user-2",
			}
			err = dbq.CreateClusterUser(ctx, testClusterUser)
			Expect(err).ToNot(HaveOccurred())

			applicationSecond := db.Application{
				Application_id:          "test-my-application-2",
				Name:                    "my-application",
				Spec_field:              "{}",
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironment.Managedenvironment_id,
			}

			err = dbq.CreateApplication(ctx, &applicationSecond)
			Expect(err).ToNot(HaveOccurred())

			operationSecond := &db.Operation{
				Operation_id:            "test-operation-2",
				Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
				Resource_id:             "fake resource id",
				Resource_type:           "GitopsEngineInstance",
				State:                   db.OperationState_Waiting,
				Operation_owner_user_id: testClusterUser.Clusteruser_id,
			}

			err = dbq.CreateOperation(ctx, operationSecond, operationSecond.Operation_owner_user_id)
			Expect(err).ToNot(HaveOccurred())

			syncoperationSecond := db.SyncOperation{
				SyncOperation_id:    "test-sync-2",
				Application_id:      applicationFirst.Application_id,
				DeploymentNameField: "testDeployment",
				Revision:            "testRev",
				DesiredState:        "Terminated",
			}

			err = dbq.CreateSyncOperation(ctx, &syncoperationSecond)
			Expect(err).ToNot(HaveOccurred())

			rowsAffected, err := dbq.DeleteSyncOperationById(ctx, syncoperationSecond.SyncOperation_id)
			Expect(err).ToNot(HaveOccurred())
			Expect(rowsAffected).Should(Equal(1))

			err = dbq.GetSyncOperationById(ctx, &syncoperationFirst)
			Expect(err).ToNot(HaveOccurred())

			err = dbq.GetSyncOperationById(ctx, &syncoperationSecond)
			Expect(true).To(Equal(db.IsResultNotFoundError(err)))

		})

		It("Should test guard row against update and delete for repo creds", func() {
			_, _, _, gitopsEngineInstance, _, err := db.CreateSampleData(dbq)
			Expect(err).ToNot(HaveOccurred())

			var testClusterUser = &db.ClusterUser{
				Clusteruser_id: "test-user-1",
				User_name:      "test-user-1",
			}
			err = dbq.CreateClusterUser(ctx, testClusterUser)
			Expect(err).ToNot(HaveOccurred())

			gitopsRepositoryCredentialsFirst := db.RepositoryCredentials{
				RepositoryCredentialsID: "test-repo-cred-id",
				UserID:                  testClusterUser.Clusteruser_id, // constrain 'fk_clusteruser_id'
				PrivateURL:              "https://test-private-url",
				AuthUsername:            "test-auth-username",
				AuthPassword:            "test-auth-password",
				AuthSSHKey:              "test-auth-ssh-key",
				SecretObj:               "test-secret-obj",
				EngineClusterID:         gitopsEngineInstance.Gitopsengineinstance_id, // constrain 'fk_gitopsengineinstance_id'
			}
			err = dbq.CreateRepositoryCredentials(ctx, &gitopsRepositoryCredentialsFirst)
			Expect(err).ToNot(HaveOccurred())

			testClusterUser = &db.ClusterUser{
				Clusteruser_id: "test-user-2",
				User_name:      "test-user-2",
			}
			err = dbq.CreateClusterUser(ctx, testClusterUser)
			Expect(err).ToNot(HaveOccurred())

			gitopsRepositoryCredentialsSecond := db.RepositoryCredentials{
				RepositoryCredentialsID: "test-repo-cred-id-2",
				UserID:                  testClusterUser.Clusteruser_id, // constrain 'fk_clusteruser_id'
				PrivateURL:              "https://test-private-url-2",
				AuthUsername:            "test-auth-username-2",
				AuthPassword:            "test-auth-password-2",
				AuthSSHKey:              "test-auth-ssh-key-2",
				SecretObj:               "test-secret-obj-2",
				EngineClusterID:         gitopsEngineInstance.Gitopsengineinstance_id, // constrain 'fk_gitopsengineinstance_id'
			}
			err = dbq.CreateRepositoryCredentials(ctx, &gitopsRepositoryCredentialsSecond)
			Expect(err).ToNot(HaveOccurred())

			fetch, err := dbq.GetRepositoryCredentialsByID(ctx, gitopsRepositoryCredentialsSecond.RepositoryCredentialsID)
			Expect(err).ToNot(HaveOccurred())

			gitopsRepositoryCredentialsSecond = fetch
			gitopsRepositoryCredentialsSecond.AuthUsername = "updated-auth-username"

			err = dbq.UpdateRepositoryCredentials(ctx, &gitopsRepositoryCredentialsSecond)
			Expect(err).ToNot(HaveOccurred())

			gitopsRepositoryCredentialsFirst, err = dbq.GetRepositoryCredentialsByID(ctx, gitopsRepositoryCredentialsFirst.RepositoryCredentialsID)
			Expect(err).ToNot(HaveOccurred())
			gitopsRepositoryCredentialsSecond, err = dbq.GetRepositoryCredentialsByID(ctx, gitopsRepositoryCredentialsSecond.RepositoryCredentialsID)
			Expect(err).ToNot(HaveOccurred())

			Expect(gitopsRepositoryCredentialsSecond.AuthUsername).Should(Equal("updated-auth-username"))
			Expect(gitopsRepositoryCredentialsFirst.AuthUsername).ShouldNot(Equal(gitopsRepositoryCredentialsSecond.AuthUsername))

			rowsAffected, err := dbq.DeleteRepositoryCredentialsByID(ctx, gitopsRepositoryCredentialsSecond.RepositoryCredentialsID)
			Expect(err).ToNot(HaveOccurred())
			Expect(rowsAffected).Should(Equal(1))

			_, err = dbq.GetRepositoryCredentialsByID(ctx, gitopsRepositoryCredentialsFirst.RepositoryCredentialsID)
			Expect(err).ToNot(HaveOccurred())

			_, err = dbq.GetRepositoryCredentialsByID(ctx, gitopsRepositoryCredentialsSecond.RepositoryCredentialsID)
			Expect(true).To(Equal(db.IsResultNotFoundError(err)))

		})
	})
})
