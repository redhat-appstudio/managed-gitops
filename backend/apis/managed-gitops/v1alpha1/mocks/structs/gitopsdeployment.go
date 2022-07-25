package structs

import (
	"fmt"

	api "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	TestGitopsDeploymentName string = "fakeGitopsDeployment"
	TestNamespace            string = "fakeNamespace"
)

type testGitopsDeploymentBuilder struct {
	p api.GitOpsDeployment
}

func (t *testGitopsDeploymentBuilder) GetGitopsDeployment() *api.GitOpsDeployment {
	return &t.p
}

func NewGitOpsDeploymentBuilder() *testGitopsDeploymentBuilder {
	return &testGitopsDeploymentBuilder{
		p: api.GitOpsDeployment{
			TypeMeta: metav1.TypeMeta{},
			ObjectMeta: metav1.ObjectMeta{
				Name:      TestGitopsDeploymentName,
				Namespace: TestNamespace,
			},
			Spec: api.GitOpsDeploymentSpec{
				Source: api.ApplicationSource{
					RepoURL: "fakeSourceRepoURL",
				},
			},
		},
	}
}

func (t *testGitopsDeploymentBuilder) Initialized() *testGitopsDeploymentBuilder {
	t.p.Status = api.GitOpsDeploymentStatus{
		Conditions: []api.GitOpsDeploymentCondition{},
	}
	return t
}

func (t *testGitopsDeploymentBuilder) WithFinalizer(finalizers []string) *testGitopsDeploymentBuilder {
	t.p.Finalizers = finalizers
	return t
}

type GitOpsDeploymentMatcher struct {
	ActualGitOpsDeployment *api.GitOpsDeployment
	FailReason             string
}

func NewGitopsDeploymentMatcher() *GitOpsDeploymentMatcher {
	return &GitOpsDeploymentMatcher{
		ActualGitOpsDeployment: &api.GitOpsDeployment{},
		FailReason:             "",
	}
}

func (m *GitOpsDeploymentMatcher) Matches(x interface{}) bool {
	ref, isCorrectType := x.(*api.GitOpsDeployment)
	if !isCorrectType {
		m.FailReason = fmt.Sprintf("Unexpected type passed: want '%T', got '%T'", api.GitOpsDeployment{}, x)
		return false
	}
	m.ActualGitOpsDeployment = ref
	return true
}

func (m *GitOpsDeploymentMatcher) String() string {
	return "Fail reason: " + m.FailReason
}
