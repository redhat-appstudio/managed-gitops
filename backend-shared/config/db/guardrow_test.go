package db_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	db "github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
)

var _ = Describe("GuardRow test", func() {
	FContext("It creates database unit tests which guard against missing WHERE clauses of UPDATE/DELETE operations to the database ", func() {

		It("Should test guard row against delete for ApiCRtoDBmapping", func() {
			err := db.SetupForTestingDBGinkgo()
			Expect(err).To(BeNil())

			ctx := context.Background()
			dbq, err := db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).To(BeNil())
			defer dbq.CloseDatabase()

			Apicrtodatabasemappingfirst := db.APICRToDatabaseMapping{
				APIResourceType:      db.APICRToDatabaseMapping_ResourceType_GitOpsDeploymentSyncRun,
				APIResourceUID:       "test-k8s-uid",
				APIResourceName:      "test-k8s-name",
				APIResourceNamespace: "test-k8s-namespace",
				NamespaceUID:         "test-namespace-uid",
				DBRelationType:       db.APICRToDatabaseMapping_DBRelationType_SyncOperation,
				DBRelationKey:        "test-key",
			}

			err = dbq.CreateAPICRToDatabaseMapping(ctx, &Apicrtodatabasemappingfirst)
			Expect(err).To(BeNil())

			Apicrtodatabasemappingsecond := db.APICRToDatabaseMapping{
				APIResourceType:      "test-GitOpsDeployment",
				APIResourceUID:       "test-k8s-uid-second",
				APIResourceName:      "test-k8s-name",
				APIResourceNamespace: "test-k8s-namespace",
				NamespaceUID:         "test-namespace-uid",
				DBRelationType:       "test-sync-operation",
				DBRelationKey:        "test-key-second",
			}
			err = dbq.CreateAPICRToDatabaseMapping(ctx, &Apicrtodatabasemappingsecond)
			Expect(err).To(BeNil())

			rowsAffected, err := dbq.DeleteAPICRToDatabaseMapping(ctx, &Apicrtodatabasemappingsecond)
			Expect(err).To(BeNil())
			Expect(rowsAffected).To(Equal((1)))

			err = dbq.GetDatabaseMappingForAPICR(ctx, &Apicrtodatabasemappingfirst)
			Expect(err).To(BeNil())

			err = dbq.GetDatabaseMappingForAPICR(ctx, &Apicrtodatabasemappingsecond)
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

			applicationfirst := db.Application{
				Application_id:          "test-my-application-1",
				Name:                    "my-application",
				Spec_field:              "{}",
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironment.Managedenvironment_id,
			}

			err = dbq.CreateApplication(ctx, &applicationfirst)
			Expect(err).To(BeNil())

			applicationsecond := db.Application{
				Application_id:          "test-my-application-2",
				Name:                    "my-application",
				Spec_field:              "{}",
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironment.Managedenvironment_id,
			}
			err = dbq.CreateApplication(ctx, &applicationsecond)
			Expect(err).To(BeNil())

			applicationsecond = db.Application{
				Application_id:          applicationsecond.Application_id,
				Name:                    "test-application-update",
				Spec_field:              "{}",
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironment.Managedenvironment_id,
				SeqID:                   applicationsecond.SeqID,
			}

			err = dbq.UpdateApplication(ctx, &applicationsecond)
			Expect(err).To(BeNil())
			Expect(applicationsecond.Name).Should(Equal("test-application-update"))
			Expect(applicationfirst.Name).ShouldNot(Equal(applicationsecond.Name))

			rowsAffected, err := dbq.DeleteApplicationById(ctx, applicationsecond.Application_id)
			Expect(err).To(BeNil())
			Expect(rowsAffected).Should(Equal(1))

			err = dbq.GetApplicationById(ctx, &applicationfirst)
			Expect(err).To(BeNil())

			err = dbq.GetApplicationById(ctx, &applicationsecond)
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

			applicationfirst := db.Application{
				Application_id:          "test-my-application-1",
				Name:                    "my-application",
				Spec_field:              "{}",
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironment.Managedenvironment_id,
			}

			err = dbq.CreateApplication(ctx, &applicationfirst)
			Expect(err).To(BeNil())

			applicationStatefirst := &db.ApplicationState{
				Applicationstate_application_id: applicationfirst.Application_id,
				Health:                          "Progressing",
				Sync_Status:                     "Unknown",
				Resources:                       make([]byte, 10),
			}

			err = dbq.CreateApplicationState(ctx, applicationStatefirst)
			Expect(err).To(BeNil())

			applicationsecond := db.Application{
				Application_id:          "test-my-application-2",
				Name:                    "my-application",
				Spec_field:              "{}",
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironment.Managedenvironment_id,
			}
			err = dbq.CreateApplication(ctx, &applicationsecond)
			Expect(err).To(BeNil())

			applicationStatesecond := &db.ApplicationState{
				Applicationstate_application_id: applicationsecond.Application_id,
				Health:                          "Progressing",
				Sync_Status:                     "Unknown",
				Resources:                       make([]byte, 10),
			}

			err = dbq.CreateApplicationState(ctx, applicationStatesecond)
			Expect(err).To(BeNil())
			applicationStatesecond = &db.ApplicationState{
				Applicationstate_application_id: applicationsecond.Application_id,
				Health:                          "Progressing",
				Sync_Status:                     "Sync",
				Resources:                       make([]byte, 10),
			}

			err = dbq.UpdateApplicationState(ctx, applicationStatesecond)
			Expect(err).To(BeNil())
			Expect(applicationStatesecond.Sync_Status).Should(Equal("Sync"))
			Expect(applicationStatefirst.Sync_Status).ShouldNot(Equal(applicationStatesecond.Sync_Status))

			rowsAffected, err := dbq.DeleteApplicationStateById(ctx, applicationStatesecond.Applicationstate_application_id)
			Expect(err).To(BeNil())
			Expect(rowsAffected).Should(Equal(1))

			err = dbq.GetApplicationStateById(ctx, applicationStatefirst)
			Expect(err).To(BeNil())

			err = dbq.GetApplicationStateById(ctx, applicationStatesecond)
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
				Clusteruser_id: "test-user-1",
				User_name:      "test-user-1",
			}
			err = dbq.CreateClusterUser(ctx, clusterUser)
			Expect(err).To(BeNil())

			clusterCredentialsfirst := db.ClusterCredentials{
				Clustercredentials_cred_id:  "test-cluster-creds-test-1",
				Host:                        "host",
				Kube_config:                 "kube-config",
				Kube_config_context:         "kube-config-context",
				Serviceaccount_bearer_token: "serviceaccount_bearer_token",
				Serviceaccount_ns:           "Serviceaccount_ns",
			}

			managedEnvironmentfirst := db.ManagedEnvironment{
				Managedenvironment_id: "test-managed-env-1",
				Clustercredentials_id: clusterCredentialsfirst.Clustercredentials_cred_id,
				Name:                  "my env",
			}

			gitopsEngineClusterfirst := db.GitopsEngineCluster{
				Gitopsenginecluster_id: "test-fake-cluster-1",
				Clustercredentials_id:  clusterCredentialsfirst.Clustercredentials_cred_id,
			}

			gitopsEngineInstancefirst := db.GitopsEngineInstance{
				Gitopsengineinstance_id: "test-fake-engine-instance-id-1",
				Namespace_name:          "test-fake-namespace",
				Namespace_uid:           "test-fake-namespace-5",
				EngineCluster_id:        gitopsEngineClusterfirst.Gitopsenginecluster_id,
			}

			clusterAccessfirst := db.ClusterAccess{
				Clusteraccess_user_id:                   clusterUser.Clusteruser_id,
				Clusteraccess_managed_environment_id:    managedEnvironmentfirst.Managedenvironment_id,
				Clusteraccess_gitops_engine_instance_id: gitopsEngineInstancefirst.Gitopsengineinstance_id,
			}

			err = dbq.CreateClusterCredentials(ctx, &clusterCredentialsfirst)
			Expect(err).To(BeNil())

			err = dbq.CreateManagedEnvironment(ctx, &managedEnvironmentfirst)
			Expect(err).To(BeNil())

			err = dbq.CreateGitopsEngineCluster(ctx, &gitopsEngineClusterfirst)
			Expect(err).To(BeNil())

			err = dbq.CreateGitopsEngineInstance(ctx, &gitopsEngineInstancefirst)
			Expect(err).To(BeNil())

			err = dbq.CreateClusterAccess(ctx, &clusterAccessfirst)
			Expect(err).To(BeNil())

			clusterUser = &db.ClusterUser{
				Clusteruser_id: "test-user-2",
				User_name:      "test-user-2",
			}
			err = dbq.CreateClusterUser(ctx, clusterUser)
			Expect(err).To(BeNil())

			clusterCredentialssecond := db.ClusterCredentials{
				Clustercredentials_cred_id:  "test-cluster-creds-test-2",
				Host:                        "host",
				Kube_config:                 "kube-config",
				Kube_config_context:         "kube-config-context",
				Serviceaccount_bearer_token: "serviceaccount_bearer_token",
				Serviceaccount_ns:           "Serviceaccount_ns",
			}

			managedEnvironmentsecond := db.ManagedEnvironment{
				Managedenvironment_id: "test-managed-env-2",
				Clustercredentials_id: clusterCredentialssecond.Clustercredentials_cred_id,
				Name:                  "my env",
			}

			gitopsEngineClustersecond := db.GitopsEngineCluster{
				Gitopsenginecluster_id: "test-fake-cluster-2",
				Clustercredentials_id:  clusterCredentialssecond.Clustercredentials_cred_id,
			}

			gitopsEngineInstancesecond := db.GitopsEngineInstance{
				Gitopsengineinstance_id: "test-fake-engine-instance-id-2",
				Namespace_name:          "test-fake-namespace",
				Namespace_uid:           "test-fake-namespace-5",
				EngineCluster_id:        gitopsEngineClustersecond.Gitopsenginecluster_id,
			}

			clusterAccesssecond := db.ClusterAccess{
				Clusteraccess_user_id:                   clusterUser.Clusteruser_id,
				Clusteraccess_managed_environment_id:    managedEnvironmentsecond.Managedenvironment_id,
				Clusteraccess_gitops_engine_instance_id: gitopsEngineInstancesecond.Gitopsengineinstance_id,
			}

			err = dbq.CreateClusterCredentials(ctx, &clusterCredentialssecond)
			Expect(err).To(BeNil())

			err = dbq.CreateManagedEnvironment(ctx, &managedEnvironmentsecond)
			Expect(err).To(BeNil())

			err = dbq.CreateGitopsEngineCluster(ctx, &gitopsEngineClustersecond)
			Expect(err).To(BeNil())

			err = dbq.CreateGitopsEngineInstance(ctx, &gitopsEngineInstancesecond)
			Expect(err).To(BeNil())

			err = dbq.CreateClusterAccess(ctx, &clusterAccesssecond)
			Expect(err).To(BeNil())

			affectedRows, err := dbq.DeleteClusterAccessById(ctx, clusterAccesssecond.Clusteraccess_user_id, clusterAccesssecond.Clusteraccess_managed_environment_id, clusterAccesssecond.Clusteraccess_gitops_engine_instance_id)
			Expect(err).To(BeNil())
			Expect(affectedRows).To(Equal(1))

			err = dbq.GetClusterAccessByPrimaryKey(ctx, &clusterAccessfirst)
			Expect(err).To(BeNil())

			err = dbq.GetClusterAccessByPrimaryKey(ctx, &clusterAccesssecond)
			Expect(true).To(Equal(db.IsResultNotFoundError(err)))

		})

		It("Should test guard row against delete for clustercredentials", func() {
			err := db.SetupForTestingDBGinkgo()
			Expect(err).To(BeNil())

			ctx := context.Background()
			dbq, err := db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).To(BeNil())
			defer dbq.CloseDatabase()

			clusterCredfirst := db.ClusterCredentials{
				Host:                        "test-host",
				Kube_config:                 "test-kube_config",
				Kube_config_context:         "test-kube_config_context",
				Serviceaccount_bearer_token: "test-serviceaccount_bearer_token",
				Serviceaccount_ns:           "test-serviceaccount_ns",
			}
			err = dbq.CreateClusterCredentials(ctx, &clusterCredfirst)
			Expect(err).To(BeNil())

			clusterCredsecond := db.ClusterCredentials{
				Host:                        "test-host",
				Kube_config:                 "test-kube_config",
				Kube_config_context:         "test-kube_config_context",
				Serviceaccount_bearer_token: "test-serviceaccount_bearer_token",
				Serviceaccount_ns:           "test-serviceaccount_ns",
			}
			err = dbq.CreateClusterCredentials(ctx, &clusterCredsecond)
			Expect(err).To(BeNil())

			rowsAffected, err := dbq.DeleteClusterCredentialsById(ctx, clusterCredsecond.Clustercredentials_cred_id)
			Expect(err).To(BeNil())
			Expect(rowsAffected).Should(Equal(1))

			err = dbq.GetClusterCredentialsById(ctx, &clusterCredfirst)
			Expect(err).To(BeNil())

			err = dbq.GetClusterCredentialsById(ctx, &clusterCredsecond)
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
				Clusteruser_id: "test-user-id-1",
				User_name:      "test-user-1",
			}
			err = dbq.CreateClusterUser(ctx, userfirst)
			Expect(err).To(BeNil())

			usersecond := &db.ClusterUser{
				Clusteruser_id: "test-user-id-2",
				User_name:      "test-user-2",
			}
			err = dbq.CreateClusterUser(ctx, usersecond)
			Expect(err).To(BeNil())

			rowsAffected, err := dbq.DeleteClusterUserById(ctx, usersecond.Clusteruser_id)
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

			applicationfirst := db.Application{
				Application_id:          "test-my-application-1",
				Name:                    "my-application",
				Spec_field:              "{}",
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironment.Managedenvironment_id,
			}

			err = dbq.CreateApplication(ctx, &applicationfirst)
			Expect(err).To(BeNil())

			deploymentToApplicationMappingfirst := &db.DeploymentToApplicationMapping{
				Deploymenttoapplicationmapping_uid_id: "test-" + generateUuid(),
				Application_id:                        applicationfirst.Application_id,
				DeploymentName:                        "test-deployment",
				DeploymentNamespace:                   "test-namespace",
				NamespaceUID:                          "demo-namespace",
			}

			err = dbq.CreateDeploymentToApplicationMapping(ctx, deploymentToApplicationMappingfirst)
			Expect(err).To(BeNil())

			applicationsecond := db.Application{
				Application_id:          "test-my-application-2",
				Name:                    "my-application",
				Spec_field:              "{}",
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironment.Managedenvironment_id,
			}
			err = dbq.CreateApplication(ctx, &applicationsecond)
			Expect(err).To(BeNil())

			deploymentToApplicationMappingsecond := &db.DeploymentToApplicationMapping{
				Deploymenttoapplicationmapping_uid_id: "test-" + generateUuid(),
				Application_id:                        applicationsecond.Application_id,
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

			clusterCredentialsfirst := db.ClusterCredentials{
				Clustercredentials_cred_id:  "test-cluster-creds-test-1",
				Host:                        "host",
				Kube_config:                 "kube-config",
				Kube_config_context:         "kube-config-context",
				Serviceaccount_bearer_token: "serviceaccount_bearer_token",
				Serviceaccount_ns:           "Serviceaccount_ns",
			}

			gitopsEngineClusterfirst := db.GitopsEngineCluster{
				Gitopsenginecluster_id: "test-fake-cluster-1",
				Clustercredentials_id:  clusterCredentialsfirst.Clustercredentials_cred_id,
			}

			err = dbq.CreateClusterCredentials(ctx, &clusterCredentialsfirst)
			Expect(err).To(BeNil())

			err = dbq.CreateGitopsEngineCluster(ctx, &gitopsEngineClusterfirst)
			Expect(err).To(BeNil())

			clusterCredentialssecond := db.ClusterCredentials{
				Clustercredentials_cred_id:  "test-cluster-creds-test-2",
				Host:                        "host",
				Kube_config:                 "kube-config",
				Kube_config_context:         "kube-config-context",
				Serviceaccount_bearer_token: "serviceaccount_bearer_token",
				Serviceaccount_ns:           "Serviceaccount_ns",
			}

			gitopsEngineClustersecond := db.GitopsEngineCluster{
				Gitopsenginecluster_id: "test-fake-cluster-2",
				Clustercredentials_id:  clusterCredentialssecond.Clustercredentials_cred_id,
			}

			err = dbq.CreateClusterCredentials(ctx, &clusterCredentialssecond)
			Expect(err).To(BeNil())

			err = dbq.CreateGitopsEngineCluster(ctx, &gitopsEngineClustersecond)
			Expect(err).To(BeNil())

			rowsAffected, err := dbq.DeleteGitopsEngineClusterById(ctx, gitopsEngineClustersecond.Gitopsenginecluster_id)
			Expect(err).To(BeNil())
			Expect(rowsAffected).Should(Equal(1))

			err = dbq.GetGitopsEngineClusterById(ctx, &gitopsEngineClusterfirst)
			Expect(err).To(BeNil())

			err = dbq.GetGitopsEngineClusterById(ctx, &gitopsEngineClustersecond)
			Expect(true).To(Equal(db.IsResultNotFoundError(err)))
		})

		It("Should test guard row against delete for gitopsengineinstance", func() {
			err := db.SetupForTestingDBGinkgo()
			Expect(err).To(BeNil())

			ctx := context.Background()
			dbq, err := db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).To(BeNil())
			defer dbq.CloseDatabase()

			clusterCredentialsfirst := db.ClusterCredentials{
				Clustercredentials_cred_id:  "test-cluster-creds-test-1",
				Host:                        "host",
				Kube_config:                 "kube-config",
				Kube_config_context:         "kube-config-context",
				Serviceaccount_bearer_token: "serviceaccount_bearer_token",
				Serviceaccount_ns:           "Serviceaccount_ns",
			}

			gitopsEngineClusterfirst := db.GitopsEngineCluster{
				Gitopsenginecluster_id: "test-fake-cluster-1",
				Clustercredentials_id:  clusterCredentialsfirst.Clustercredentials_cred_id,
			}

			gitopsEngineInstancefirst := db.GitopsEngineInstance{
				Gitopsengineinstance_id: "test-fake-engine-instance-id-1",
				Namespace_name:          "test-fake-namespace",
				Namespace_uid:           "test-fake-namespace-1",
				EngineCluster_id:        gitopsEngineClusterfirst.Gitopsenginecluster_id,
			}
			err = dbq.CreateClusterCredentials(ctx, &clusterCredentialsfirst)
			Expect(err).To(BeNil())

			err = dbq.CreateGitopsEngineCluster(ctx, &gitopsEngineClusterfirst)
			Expect(err).To(BeNil())

			err = dbq.CreateGitopsEngineInstance(ctx, &gitopsEngineInstancefirst)
			Expect(err).To(BeNil())

			clusterCredentialssecond := db.ClusterCredentials{
				Clustercredentials_cred_id:  "test-cluster-creds-test-2",
				Host:                        "host",
				Kube_config:                 "kube-config",
				Kube_config_context:         "kube-config-context",
				Serviceaccount_bearer_token: "serviceaccount_bearer_token",
				Serviceaccount_ns:           "Serviceaccount_ns",
			}

			gitopsEngineClustersecond := db.GitopsEngineCluster{
				Gitopsenginecluster_id: "test-fake-cluster-2",
				Clustercredentials_id:  clusterCredentialssecond.Clustercredentials_cred_id,
			}

			gitopsEngineInstancesecond := db.GitopsEngineInstance{
				Gitopsengineinstance_id: "test-fake-engine-instance-id-2",
				Namespace_name:          "test-fake-namespace",
				Namespace_uid:           "test-fake-namespace-1",
				EngineCluster_id:        gitopsEngineClustersecond.Gitopsenginecluster_id,
			}
			err = dbq.CreateClusterCredentials(ctx, &clusterCredentialssecond)
			Expect(err).To(BeNil())

			err = dbq.CreateGitopsEngineCluster(ctx, &gitopsEngineClustersecond)
			Expect(err).To(BeNil())

			err = dbq.CreateGitopsEngineInstance(ctx, &gitopsEngineInstancesecond)
			Expect(err).To(BeNil())

			rowsAffected, err := dbq.DeleteGitopsEngineInstanceById(ctx, gitopsEngineInstancesecond.Gitopsengineinstance_id)
			Expect(err).To(BeNil())
			Expect(rowsAffected).Should(Equal(1))

			err = dbq.GetGitopsEngineInstanceById(ctx, &gitopsEngineInstancefirst)
			Expect(err).To(BeNil())

			err = dbq.GetGitopsEngineInstanceById(ctx, &gitopsEngineInstancesecond)
			Expect(true).To(Equal(db.IsResultNotFoundError(err)))
		})

		It("Should test guard row against delete on k8stodbmapping", func() {
			err := db.SetupForTestingDBGinkgo()
			Expect(err).To(BeNil())

			ctx := context.Background()
			dbq, err := db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).To(BeNil())
			defer dbq.CloseDatabase()

			kubernetesToDBResourceMappingfirst := db.KubernetesToDBResourceMapping{
				KubernetesResourceType: "test-resource_1",
				KubernetesResourceUID:  "test-resource_uid",
				DBRelationType:         "test-relation_type",
				DBRelationKey:          "test-relation_key",
			}
			err = dbq.CreateKubernetesResourceToDBResourceMapping(ctx, &kubernetesToDBResourceMappingfirst)
			Expect(err).To(BeNil())

			kubernetesToDBResourceMappingsecond := db.KubernetesToDBResourceMapping{
				KubernetesResourceType: "test-resource_2",
				KubernetesResourceUID:  "test-resource_uid",
				DBRelationType:         "test-relation_type",
				DBRelationKey:          "test-relation_key",
			}
			err = dbq.CreateKubernetesResourceToDBResourceMapping(ctx, &kubernetesToDBResourceMappingsecond)
			Expect(err).To(BeNil())

			rowsAffected, err := dbq.DeleteKubernetesResourceToDBResourceMapping(ctx, &kubernetesToDBResourceMappingsecond)
			Expect(err).To(BeNil())
			Expect(rowsAffected).Should(Equal(1))

			err = dbq.GetDBResourceMappingForKubernetesResource(ctx, &kubernetesToDBResourceMappingfirst)
			Expect(err).To(BeNil())

			err = dbq.GetDBResourceMappingForKubernetesResource(ctx, &kubernetesToDBResourceMappingsecond)
			Expect(true).To(Equal(db.IsResultNotFoundError(err)))

		})

		It("Should test guard row against delete on managedenvironment", func() {
			err := db.SetupForTestingDBGinkgo()
			Expect(err).To(BeNil())

			ctx := context.Background()
			dbq, err := db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).To(BeNil())
			defer dbq.CloseDatabase()

			clusterCredentialsfirst := db.ClusterCredentials{
				Clustercredentials_cred_id:  "test-cluster-creds-test-1",
				Host:                        "host",
				Kube_config:                 "kube-config",
				Kube_config_context:         "kube-config-context",
				Serviceaccount_bearer_token: "serviceaccount_bearer_token",
				Serviceaccount_ns:           "Serviceaccount_ns",
			}

			managedEnvironmentfirst := db.ManagedEnvironment{
				Managedenvironment_id: "test-managed-env-1",
				Clustercredentials_id: clusterCredentialsfirst.Clustercredentials_cred_id,
				Name:                  "my env101",
			}

			err = dbq.CreateClusterCredentials(ctx, &clusterCredentialsfirst)
			Expect(err).To(BeNil())

			err = dbq.CreateManagedEnvironment(ctx, &managedEnvironmentfirst)
			Expect(err).To(BeNil())

			clusterCredentialssecond := db.ClusterCredentials{
				Clustercredentials_cred_id:  "test-cluster-creds-test-2",
				Host:                        "host",
				Kube_config:                 "kube-config",
				Kube_config_context:         "kube-config-context",
				Serviceaccount_bearer_token: "serviceaccount_bearer_token",
				Serviceaccount_ns:           "Serviceaccount_ns",
			}

			managedEnvironmentsecond := db.ManagedEnvironment{
				Managedenvironment_id: "test-managed-env-2",
				Clustercredentials_id: clusterCredentialssecond.Clustercredentials_cred_id,
				Name:                  "my env101",
			}

			err = dbq.CreateClusterCredentials(ctx, &clusterCredentialssecond)
			Expect(err).To(BeNil())

			err = dbq.CreateManagedEnvironment(ctx, &managedEnvironmentsecond)
			Expect(err).To(BeNil())

			rowsAffected, err := dbq.DeleteManagedEnvironmentById(ctx, managedEnvironmentsecond.Managedenvironment_id)
			Expect(err).To(BeNil())
			Expect(rowsAffected).Should(Equal(1))

			err = dbq.GetManagedEnvironmentById(ctx, &managedEnvironmentfirst)
			Expect(err).To(BeNil())

			err = dbq.GetManagedEnvironmentById(ctx, &managedEnvironmentsecond)
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
				Clusteruser_id: "test-user-1",
				User_name:      "test-user-1",
			}
			operationfirst := db.Operation{
				Operation_id:            "test-operation-1",
				Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
				Resource_id:             "test-fake-resource-id",
				Resource_type:           "GitopsEngineInstance",
				State:                   db.OperationState_Waiting,
				Operation_owner_user_id: testClusterUser.Clusteruser_id,
			}
			err = dbq.CreateClusterUser(ctx, testClusterUser)
			Expect(err).To(BeNil())

			err = dbq.CreateOperation(ctx, &operationfirst, operationfirst.Operation_owner_user_id)
			Expect(err).To(BeNil())

			testClusterUser = &db.ClusterUser{
				Clusteruser_id: "test-user-2",
				User_name:      "test-user-2",
			}
			operationsecond := db.Operation{
				Operation_id:            "test-operation-2",
				Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
				Resource_id:             "test-fake-resource-id",
				Resource_type:           "GitopsEngineInstance",
				State:                   db.OperationState_Waiting,
				Operation_owner_user_id: testClusterUser.Clusteruser_id,
			}
			err = dbq.CreateClusterUser(ctx, testClusterUser)
			Expect(err).To(BeNil())

			err = dbq.CreateOperation(ctx, &operationsecond, operationsecond.Operation_owner_user_id)
			Expect(err).To(BeNil())

			operationsecond = db.Operation{
				Operation_id:            "test-operation-2",
				Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
				Resource_id:             "test-fake-resource-id-update",
				Resource_type:           "GitopsEngineInstance",
				State:                   db.OperationState_Waiting,
				Operation_owner_user_id: testClusterUser.Clusteruser_id,
				SeqID:                   operationsecond.SeqID,
				Created_on:              operationsecond.Created_on,
				Last_state_update:       operationsecond.Last_state_update,
			}

			err = dbq.UpdateOperation(ctx, &operationsecond)
			Expect(err).To(BeNil())
			Expect(operationsecond.Resource_id).Should(Equal("test-fake-resource-id-update"))
			Expect(operationfirst.Resource_id).ShouldNot(Equal(operationsecond.Resource_id))

			rowsAffected, err := dbq.DeleteOperationById(ctx, operationsecond.Operation_id)
			Expect(err).To(BeNil())
			Expect(rowsAffected).Should(Equal(1))

			err = dbq.GetOperationById(ctx, &operationfirst)
			Expect(err).To(BeNil())

			err = dbq.GetOperationById(ctx, &operationsecond)
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
				Clusteruser_id: "test-user-1",
				User_name:      "test-user-1",
			}
			err = dbq.CreateClusterUser(ctx, testClusterUser)
			Expect(err).To(BeNil())
			applicationfirst := db.Application{
				Application_id:          "test-my-application-1",
				Name:                    "my-application",
				Spec_field:              "{}",
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironment.Managedenvironment_id,
			}

			err = dbq.CreateApplication(ctx, &applicationfirst)
			Expect(err).To(BeNil())

			operationfirst := &db.Operation{
				Operation_id:            "test-operation-1",
				Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
				Resource_id:             "fake resource id",
				Resource_type:           "GitopsEngineInstance",
				State:                   db.OperationState_Waiting,
				Operation_owner_user_id: testClusterUser.Clusteruser_id,
			}

			err = dbq.CreateOperation(ctx, operationfirst, operationfirst.Operation_owner_user_id)
			Expect(err).To(BeNil())

			syncoperationfirst := db.SyncOperation{
				SyncOperation_id:    "test-sync-1",
				Application_id:      applicationfirst.Application_id,
				DeploymentNameField: "testDeployment",
				Revision:            "testRev",
				DesiredState:        "Terminated",
			}

			err = dbq.CreateSyncOperation(ctx, &syncoperationfirst)
			Expect(err).To(BeNil())

			testClusterUser = &db.ClusterUser{
				Clusteruser_id: "test-user-2",
				User_name:      "test-user-2",
			}
			err = dbq.CreateClusterUser(ctx, testClusterUser)
			Expect(err).To(BeNil())

			applicationsecond := db.Application{
				Application_id:          "test-my-application-2",
				Name:                    "my-application",
				Spec_field:              "{}",
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironment.Managedenvironment_id,
			}

			err = dbq.CreateApplication(ctx, &applicationsecond)
			Expect(err).To(BeNil())

			operationsecond := &db.Operation{
				Operation_id:            "test-operation-2",
				Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
				Resource_id:             "fake resource id",
				Resource_type:           "GitopsEngineInstance",
				State:                   db.OperationState_Waiting,
				Operation_owner_user_id: testClusterUser.Clusteruser_id,
			}

			err = dbq.CreateOperation(ctx, operationsecond, operationsecond.Operation_owner_user_id)
			Expect(err).To(BeNil())

			syncoperationsecond := db.SyncOperation{
				SyncOperation_id:    "test-sync-2",
				Application_id:      applicationfirst.Application_id,
				DeploymentNameField: "testDeployment",
				Revision:            "testRev",
				DesiredState:        "Terminated",
			}

			err = dbq.CreateSyncOperation(ctx, &syncoperationsecond)
			Expect(err).To(BeNil())

			rowsAffected, err := dbq.DeleteSyncOperationById(ctx, syncoperationsecond.SyncOperation_id)
			Expect(err).To(BeNil())
			Expect(rowsAffected).Should(Equal(1))

			err = dbq.GetSyncOperationById(ctx, &syncoperationfirst)
			Expect(err).To(BeNil())

			err = dbq.GetSyncOperationById(ctx, &syncoperationsecond)
			Expect(true).To(Equal(db.IsResultNotFoundError(err)))

		})
	})
})
