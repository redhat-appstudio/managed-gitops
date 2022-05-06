package db_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	db "github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
)

var _ = Describe("Application Test", func() {

	It("Should Create, Get, Update and Delete an Application", func() {
		err := db.SetupForTestingDBGinkgo()
		Expect(err).To(BeNil())

		ctx := context.Background()
		dbq, err := db.NewUnsafePostgresDBQueries(true, true)
		Expect(err).To(BeNil())
		defer dbq.CloseDatabase()

		clusterCredentials := db.ClusterCredentials{
			Clustercredentials_cred_id:  "test-cluster-creds-test",
			Host:                        "host",
			Kube_config:                 "kube-config",
			Kube_config_context:         "kube-config-context",
			Serviceaccount_bearer_token: "serviceaccount_bearer_token",
			Serviceaccount_ns:           "Serviceaccount_ns",
		}

		managedEnvironment := db.ManagedEnvironment{
			Managedenvironment_id: "test-managed-env-914",
			Clustercredentials_id: clusterCredentials.Clustercredentials_cred_id,
			Name:                  "my env",
		}

		gitopsEngineCluster := db.GitopsEngineCluster{
			Gitopsenginecluster_id: "test-fake-cluster-914",
			Clustercredentials_id:  clusterCredentials.Clustercredentials_cred_id,
		}

		gitopsEngineInstance := db.GitopsEngineInstance{
			Gitopsengineinstance_id: "test-fake-engine-instance-id",
			Namespace_name:          "test-fake-namespace",
			Namespace_uid:           "test-fake-namespace-914",
			EngineCluster_id:        gitopsEngineCluster.Gitopsenginecluster_id,
		}

		application := db.Application{
			Application_id:          "test-my-application-1",
			Name:                    "test-application",
			Spec_field:              "{}",
			Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
			Managed_environment_id:  managedEnvironment.Managedenvironment_id,
		}

		err = dbq.CreateClusterCredentials(ctx, &clusterCredentials)
		Expect(err).To(BeNil())

		err = dbq.CreateManagedEnvironment(ctx, &managedEnvironment)
		Expect(err).To(BeNil())

		err = dbq.CreateGitopsEngineCluster(ctx, &gitopsEngineCluster)
		Expect(err).To(BeNil())

		err = dbq.CreateGitopsEngineInstance(ctx, &gitopsEngineInstance)
		Expect(err).To(BeNil())

		err = dbq.CreateApplication(ctx, &application)
		Expect(err).To(BeNil())

		applicationget := db.Application{
			Application_id: application.Application_id,
		}

		err = dbq.GetApplicationById(ctx, &applicationget)
		Expect(err).To(BeNil())
		Expect(application).Should(Equal(applicationget))

		applicationupdate := db.Application{

			Application_id:          application.Application_id,
			Name:                    "test-application-update",
			Spec_field:              "{}",
			Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
			Managed_environment_id:  managedEnvironment.Managedenvironment_id,
			SeqID:                   int64(seq),
		}

		err = dbq.UpdateApplication(ctx, &applicationupdate)
		Expect(err).To(BeNil())

		err = dbq.GetApplicationById(ctx, &applicationupdate)
		Expect(err).To(BeNil())
		Expect(applicationupdate).ShouldNot(Equal(applicationget))

		rowsAffected, err := dbq.DeleteApplicationById(ctx, application.Application_id)
		Expect(err).To(BeNil())
		Expect(rowsAffected).Should(Equal(1))

		err = dbq.GetApplicationById(ctx, &application)
		Expect(true).To(Equal(db.IsResultNotFoundError(err)))

	})
})
