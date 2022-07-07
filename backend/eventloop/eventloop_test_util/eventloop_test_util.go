package eventloop_test_util

import (
	"context"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"

	// . "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// StartServiceAccountListenerOnFakeClient waits for an ArgoCD Manager ServiceAccount token to be created,
// then creates a token secret and adds ito the ServiceAccount.
// This simulates the default K8s behaviour of ServiceAccount (which, in a unit test, we do not have)
func StartServiceAccountListenerOnFakeClient(ctx context.Context, managedEnvironmentUID string, k8sClient client.Client) {
	go func() {

		var sa *corev1.ServiceAccount

		// Wait for a ServiceAccount to be created for the given GitOpsDeploymentManagedEnvironment
		err := wait.Poll(time.Second*1, time.Hour*1, func() (bool, error) {
			// Look for ServiceAccounts in kube-system
			saList := corev1.ServiceAccountList{}
			err := k8sClient.List(ctx, &saList, &client.ListOptions{Namespace: "kube-system"})
			Expect(err).To(BeNil())

			for idx := range saList.Items {

				item := saList.Items[idx]

				sa = &item

				if strings.HasPrefix(item.Name, sharedutil.ArgoCDManagerServiceAccountPrefix) &&
					strings.Contains(item.Name, managedEnvironmentUID) {
					return true, nil
				}
			}

			return false, nil
		})
		Expect(err).To(BeNil())

		// Once the ServiceAccount was created, create the corresponding Secret and add it to the ServiceAccount
		tokenSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "token-secret-" + string(uuid.NewUUID()),
				Namespace: "kube-system",
				Annotations: map[string]string{
					corev1.ServiceAccountNameKey: sa.Name,
				},
			},
			Data: map[string][]byte{"token": ([]byte)("token")},
			Type: corev1.SecretTypeServiceAccountToken,
		}
		err = k8sClient.Create(ctx, tokenSecret)
		Expect(err).To(BeNil())

		sa.Secrets = append(sa.Secrets, corev1.ObjectReference{
			Name:      tokenSecret.Name,
			Namespace: tokenSecret.Namespace,
		})

		err = k8sClient.Update(ctx, sa)
		Expect(err).To(BeNil())

	}()
}
