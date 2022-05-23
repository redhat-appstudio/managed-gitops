package eventlooptypes

import (
	"testing"

	operation "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	dbutil "github.com/redhat-appstudio/managed-gitops/backend-shared/config/db/util"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend/apis/managed-gitops/v1alpha1"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/uuid"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func GenericTestSetupForTestingT(t *testing.T) (*runtime.Scheme, *v1.Namespace, *v1.Namespace, *v1.Namespace) {

	scheme, argocdNamespace, kubesystemNamespace, workspace, err := GenericTestSetup()
	assert.NoError(t, err)

	return scheme, argocdNamespace, kubesystemNamespace, workspace

}

func GenericTestSetup() (*runtime.Scheme, *v1.Namespace, *v1.Namespace, *v1.Namespace, error) {
	scheme := runtime.NewScheme()

	opts := zap.Options{
		Development: true,
	}
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	err := managedgitopsv1alpha1.AddToScheme(scheme)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	err = operation.AddToScheme(scheme)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	err = v1.AddToScheme(scheme)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	argocdNamespace := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dbutil.GetGitOpsEngineSingleInstanceNamespace(),
			UID:       uuid.NewUUID(),
			Namespace: dbutil.GetGitOpsEngineSingleInstanceNamespace(),
		},
		Spec: v1.NamespaceSpec{},
	}

	kubesystemNamespace := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kube-system",
			UID:       uuid.NewUUID(),
			Namespace: "kube-system",
		},
		Spec: v1.NamespaceSpec{},
	}

	namespace := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-user",
			UID:       uuid.NewUUID(),
			Namespace: "my-user",
		},
		Spec: v1.NamespaceSpec{},
	}

	return scheme, argocdNamespace, kubesystemNamespace, namespace, nil

}
