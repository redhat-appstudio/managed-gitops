package db_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	db "github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
)

var timestamp = time.Date(2022, time.March, 11, 12, 3, 49, 514935000, time.UTC)
var seq = 101

var _ = Describe("Operations", func() {

	It("Should Create, Get, List, Update and Delete an Operation", func() {
		err := db.SetupForTestingDBGinkgo()
		Expect(err).To(BeNil())

		ctx := context.Background()
		dbq, err := db.NewUnsafePostgresDBQueries(true, true)
		Expect(err).To(BeNil())
		defer dbq.CloseDatabase()

		var testClusterUser = &db.ClusterUser{
			Clusteruser_id: "test-user-new",
			User_name:      "test-user-new",
		}
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
		clusterAccess := db.ClusterAccess{
			Clusteraccess_user_id:                   testClusterUser.Clusteruser_id,
			Clusteraccess_managed_environment_id:    managedEnvironment.Managedenvironment_id,
			Clusteraccess_gitops_engine_instance_id: gitopsEngineInstance.Gitopsengineinstance_id,
		}

		operation := db.Operation{
			Operation_id:            "test-operation-1",
			Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
			Resource_id:             "test-fake-resource-id",
			Resource_type:           "GitopsEngineInstance",
			State:                   db.OperationState_Waiting,
			Operation_owner_user_id: testClusterUser.Clusteruser_id,
		}
		err = dbq.CreateClusterUser(ctx, testClusterUser)
		Expect(err).To(BeNil())

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

		err = dbq.CreateOperation(ctx, &operation, operation.Operation_owner_user_id)
		Expect(err).To(BeNil())

		operationget := db.Operation{
			Operation_id: operation.Operation_id,
		}

		err = dbq.GetOperationById(ctx, &operationget)
		Expect(err).To(BeNil())
		Expect(operationget.Last_state_update).To(BeAssignableToTypeOf(timestamp))
		Expect(operationget.Created_on).To(BeAssignableToTypeOf(timestamp))
		operationget.Created_on = operation.Created_on
		operationget.Last_state_update = operation.Last_state_update
		Expect(operation).Should(Equal(operationget))

		var operationlist []db.Operation

		err = dbq.ListOperationsByResourceIdAndTypeAndOwnerId(ctx, operation.Resource_id, operation.Resource_type, &operationlist, operation.Operation_owner_user_id)
		Expect(err).To(BeNil())
		Expect(operationlist[0].Last_state_update).To(BeAssignableToTypeOf(timestamp))
		Expect(operationlist[0].Created_on).To(BeAssignableToTypeOf(timestamp))
		operationlist[0].Created_on = operation.Created_on
		operationlist[0].Last_state_update = operation.Last_state_update

		Expect(operationlist[0]).Should(Equal(operation))
		Expect(len(operationlist)).Should(Equal(1))

		operationupdate := db.Operation{
			Operation_id:            "test-operation-1",
			Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
			Resource_id:             "test-fake-resource-id-update",
			Resource_type:           "GitopsEngineInstance-update",
			State:                   db.OperationState_Waiting,
			Operation_owner_user_id: testClusterUser.Clusteruser_id,
			SeqID:                   int64(seq),
		}
		operationupdate.Created_on = operation.Created_on
		operationupdate.Last_state_update = operation.Last_state_update
		err = dbq.UpdateOperation(ctx, &operationupdate)
		Expect(err).To(BeNil())
		err = dbq.GetOperationById(ctx, &operationupdate)
		Expect(err).To(BeNil())
		Expect(operationupdate).ShouldNot(Equal(operation))

		rowsAffected, err := dbq.DeleteOperationById(ctx, operationget.Operation_id)
		Expect(err).To(BeNil())
		Expect(rowsAffected).Should(Equal(1))

		err = dbq.GetOperationById(ctx, &operationget)
		Expect(true).To(Equal(db.IsResultNotFoundError(err)))
	})
})
