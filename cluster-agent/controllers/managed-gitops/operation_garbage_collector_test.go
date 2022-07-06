/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
	logger "sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("Garbage Collect Operations", func() {
	Context("Garbage Collect expired operations", func() {
		var (
			gitopsEngineInstance *db.GitopsEngineInstance

			ctx           context.Context
			dbq           db.AllDatabaseQueries
			log           logr.Logger
			gc            *garbageCollector
			clusterAccess *db.ClusterAccess
			err           error
		)

		BeforeEach(func() {
			ctx = context.Background()
			log = logger.FromContext(ctx)

			err = db.SetupForTestingDBGinkgo()
			Expect(err).To(BeNil())

			dbq, err = db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).To(BeNil())

			gc = NewGarbageCollector(dbq)

			_, _, _, gitopsEngineInstance, clusterAccess, err = db.CreateSampleData(dbq)
			Expect(err).To(BeNil())
		})

		It("operations with expired gc interval should be removed", func() {
			By("create an operation with expiration time")
			validOperation := db.Operation{
				Operation_id:            "test-operation-1",
				Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
				Resource_id:             "test-fake-resource-id",
				Resource_type:           "GitopsEngineInstance",
				State:                   db.OperationState_Waiting,
				Operation_owner_user_id: clusterAccess.Clusteraccess_user_id,
				GC_expiration_time:      2,
				Last_state_update:       time.Now(),
			}
			err = dbq.CreateOperation(ctx, &validOperation, validOperation.Operation_owner_user_id)
			Expect(err).To(BeNil())

			By("wait until we exceed the expiration time")
			time.Sleep(2 * time.Second)

			gc.garbageCollectOperations(ctx, []db.Operation{validOperation}, log)

			By("operation should be removed from DB")
			err = dbq.GetOperationById(ctx, &validOperation)
			Expect(db.IsResultNotFoundError(err)).To(BeTrue())
		})

		It("operation within the gc interval should not be removed", func() {
			By("create an operation with a long expiration time")
			invalidOperation := db.Operation{
				Operation_id:            "test-operation-1",
				Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
				Resource_id:             "test-fake-resource-id",
				Resource_type:           "GitopsEngineInstance",
				State:                   db.OperationState_Waiting,
				Operation_owner_user_id: clusterAccess.Clusteraccess_user_id,
				GC_expiration_time:      2000,
				Last_state_update:       time.Now(),
			}
			err = dbq.CreateOperation(ctx, &invalidOperation, invalidOperation.Operation_owner_user_id)
			Expect(err).To(BeNil())

			gc.garbageCollectOperations(ctx, []db.Operation{invalidOperation}, log)

			By("operation should not be removed from DB")
			err = dbq.GetOperationById(ctx, &invalidOperation)
			Expect(err).To(BeNil())

			_, err = dbq.DeleteOperationById(ctx, invalidOperation.Operation_id)
			Expect(err).To(BeNil())
		})
	})
})
