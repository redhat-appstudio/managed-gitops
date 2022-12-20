package util

import (
	"context"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/tests"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = FDescribe("Test for creating opeartion with resource-type as Gitopsengineinstance ", func() {

	Context("New Argo Instance test", func() {

		var k8sClient client.Client
		var dbQueries db.AllDatabaseQueries
		var log logr.Logger
		var ctx context.Context
		var namespace *corev1.Namespace

		// Create a fake k8s client before each test
		BeforeEach(func() {

			err := db.SetupForTestingDBGinkgo()
			Expect(err).To(BeNil())

			ctx = context.Background()
			log = logf.FromContext(ctx)

			ctx = context.Background()
			scheme,
				argocdNamespace,
				kubesystemNamespace,
				innerNamespace, err := tests.GenericTestSetup()
			Expect(err).To(BeNil())

			namespace = innerNamespace

			k8sClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(namespace, argocdNamespace, kubesystemNamespace).
				Build()

			dbQueries, err = db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).To(BeNil())

		})

		AfterEach(func() {
			dbQueries.CloseDatabase()
		})
		It("tests whether the operation pointing to gitopsengineinstance resource type gets created successfully", func() {

			clusterUser := db.ClusterUser{User_name: "gitops-service-user"}
			dbQueries.CreateClusterUser(ctx, &clusterUser)

			err := CreateNewArgoCDInstance(namespace, clusterUser, k8sClient, log, dbQueries)
			Expect(err).To(BeNil())

		})
	})
})
