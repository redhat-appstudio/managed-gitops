package eventlooptypes

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
)

var _ = Describe("eventlooptypes test", func() {
	Context("eventsMatch", func() {

		DescribeTable("eventsMatch unit tests", func(one *EventLoopEvent, two *EventLoopEvent, expectedResult string) {
			res := EventsMatch(one, two)
			Expect(res).To(Equal(expectedResult))
		},
			Entry("both params are nil", nil, nil, ""),
			Entry("first param is nil", nil, &EventLoopEvent{}, "nil mismatch"),
			Entry("second param is nil", &EventLoopEvent{}, nil, "nil mismatch"),

			Entry("mismatching event type", &EventLoopEvent{
				EventType: DeploymentModified,
			}, &EventLoopEvent{
				EventType: ManagedEnvironmentModified,
			}, "eventtype mismatch"),

			Entry("mismatching reqresource", &EventLoopEvent{
				EventType:   DeploymentModified,
				ReqResource: GitOpsDeploymentManagedEnvironmentTypeName,
			}, &EventLoopEvent{
				EventType:   DeploymentModified,
				ReqResource: GitOpsDeploymentRepositoryCredentialTypeName,
			}, "reqresource mismatch"),

			Entry("mismatching request name", &EventLoopEvent{
				EventType:   DeploymentModified,
				ReqResource: GitOpsDeploymentManagedEnvironmentTypeName,
				Request: ctrl.Request{
					NamespacedName: types.NamespacedName{Namespace: "namespace", Name: "name"},
				},
			}, &EventLoopEvent{
				EventType:   DeploymentModified,
				ReqResource: GitOpsDeploymentManagedEnvironmentTypeName,
				Request: ctrl.Request{
					NamespacedName: types.NamespacedName{Namespace: "namespace", Name: "name2"},
				},
			}, "request name mismatch"),

			Entry("mismatching request namespace", &EventLoopEvent{
				EventType:   DeploymentModified,
				ReqResource: GitOpsDeploymentManagedEnvironmentTypeName,
				Request: ctrl.Request{
					NamespacedName: types.NamespacedName{Namespace: "namespace2", Name: "name"},
				},
			}, &EventLoopEvent{
				EventType:   DeploymentModified,
				ReqResource: GitOpsDeploymentManagedEnvironmentTypeName,
				Request: ctrl.Request{
					NamespacedName: types.NamespacedName{Namespace: "namespace", Name: "name"},
				},
			}, "request namespace mismatch"),

			Entry("mismatching workspace", &EventLoopEvent{
				EventType:   DeploymentModified,
				ReqResource: GitOpsDeploymentManagedEnvironmentTypeName,
				Request: ctrl.Request{
					NamespacedName: types.NamespacedName{Namespace: "namespace", Name: "name"},
				},
				WorkspaceID: "one",
			}, &EventLoopEvent{
				EventType:   DeploymentModified,
				ReqResource: GitOpsDeploymentManagedEnvironmentTypeName,
				Request: ctrl.Request{
					NamespacedName: types.NamespacedName{Namespace: "namespace", Name: "name"},
				},
				WorkspaceID: "two",
			}, "workspaceid mismatch"),

			Entry("should match", &EventLoopEvent{
				EventType:   DeploymentModified,
				ReqResource: GitOpsDeploymentManagedEnvironmentTypeName,
				Request: ctrl.Request{
					NamespacedName: types.NamespacedName{Namespace: "namespace", Name: "name"},
				},
				WorkspaceID: "one",
			}, &EventLoopEvent{
				EventType:   DeploymentModified,
				ReqResource: GitOpsDeploymentManagedEnvironmentTypeName,
				Request: ctrl.Request{
					NamespacedName: types.NamespacedName{Namespace: "namespace", Name: "name"},
				},
				WorkspaceID: "one",
			}, ""),
		)

	})
})
