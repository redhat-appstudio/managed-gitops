package eventloop_test_util

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"

	// . "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// StartServiceAccountListenerOnFakeClient simulates the default K8s behaviour of ServiceAccountTokenSecrets on Kubernetes.
// - Wait for the ServiceAccountToken Secret to be created
// - Next, add a fake token to the secret
// - Finally, add a reference to the Secret to the ServiceAccount
func StartServiceAccountListenerOnFakeClient(ctx context.Context, managedEnvironmentUID string, k8sClient client.Client) {

	addSecretToServiceAccount := func(secret corev1.Secret) {

		// Update the token field of the Secret, to include our simulated token
		secret.Data = map[string][]byte{
			"token": ([]byte)("token"),
		}
		err := k8sClient.Update(ctx, &secret)
		Expect(err).To(BeNil())

		// Locate the ServiceAccount that is pointed to by the Secret, and update that ServiceAccount
		// - Add the Secret to the list of token secrets in the Service account
		serviceAccountName, exists := secret.Annotations[corev1.ServiceAccountNameKey]
		Expect(exists).To(BeTrue())

		if exists {

			serviceAccount := corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceAccountName,
					Namespace: secret.Namespace,
				},
			}
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(&serviceAccount), &serviceAccount)
			Expect(err).To(BeNil())

			serviceAccount.Secrets = append(serviceAccount.Secrets, corev1.ObjectReference{
				Name:      secret.Name,
				Namespace: secret.Namespace,
			})

			err = k8sClient.Update(ctx, &serviceAccount)
			Expect(err).To(BeNil())

		}
	}

	go func() {

		// Wait for the ServiceAccountToken Secret to be created
		// Then:
		// - add a fake token to the secret
		// - add a reference to the Secret to the ServiceAccount
		err := wait.Poll(time.Second*1, time.Hour*1, func() (bool, error) {

			secretList := corev1.SecretList{}
			err := k8sClient.List(ctx, &secretList, &client.ListOptions{Namespace: "kube-system"})
			Expect(err).To(BeNil())

			for idx := range secretList.Items {

				secret := secretList.Items[idx]

				if secret.Type == corev1.SecretTypeServiceAccountToken {

					// Only touch secrets that don't already have a token
					_, tokenDataExists := secret.Data["token"]
					if !tokenDataExists {
						addSecretToServiceAccount(secret)
					}
				}

			}

			return false, nil
		})
		Expect(err).To(BeNil())

	}()
}
