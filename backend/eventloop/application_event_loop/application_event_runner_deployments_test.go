package application_event_loop

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/fauxargocd"
	"gopkg.in/yaml.v2"
)

var _ = Describe("Application Event Runner Deployments", func() {
	Context("createSpecField() should generate a valid argocd Application", func() {
		getfakeArgoCDSpecInput := func(automated, unsanitized bool) argoCDSpecInput {
			input := argoCDSpecInput{
				crName:               "sample-depl",
				crNamespace:          "kcp-workspace",
				destinationNamespace: "prod",
				destinationName:      "in-cluster",
				sourceRepoURL:        "https://github.com/test/test",
				sourcePath:           "environments/prod",
				automated:            automated,
			}

			if unsanitized {
				input.sourcePath = "environments/&&prod\n"
				input.sourceRepoURL = "https://github.com/```%test/test"
			}
			return input
		}

		getValidApplication := func(automated bool) string {
			input := getfakeArgoCDSpecInput(automated, false)
			application := fauxargocd.FauxApplication{
				FauxTypeMeta: fauxargocd.FauxTypeMeta{
					Kind:       "Application",
					APIVersion: "argoproj.io/v1alpha1",
				},
				FauxObjectMeta: fauxargocd.FauxObjectMeta{
					Name:      input.crName,
					Namespace: input.crNamespace,
				},
				Spec: fauxargocd.FauxApplicationSpec{
					Source: fauxargocd.ApplicationSource{
						RepoURL:        input.sourceRepoURL,
						Path:           input.sourcePath,
						TargetRevision: input.sourceTargetRevision,
					},
					Destination: fauxargocd.ApplicationDestination{
						Name:      input.destinationName,
						Namespace: input.destinationNamespace,
					},
					Project: "default",
				},
			}
			if automated {
				application.Spec.SyncPolicy = &fauxargocd.SyncPolicy{
					Automated: &fauxargocd.SyncPolicyAutomated{
						Prune:      true,
						AllowEmpty: true,
						SelfHeal:   true,
					},
					SyncOptions: fauxargocd.SyncOptions{
						prunePropagationPolicy,
					},
				}
			}

			appBytes, err := yaml.Marshal(application)
			if err != nil {
				GinkgoT().Fatalf("failed to unmarshall Application: %q", err)
			}

			return string(appBytes)
		}

		It("Input spec is converted to an argocd Application", func() {
			input := getfakeArgoCDSpecInput(false, false)
			application, err := createSpecField(input)
			Expect(err).To(BeNil())
			Expect(application).To(Equal(getValidApplication(false)))
		})

		It("Sanitize illegal characters from input", func() {
			input := getfakeArgoCDSpecInput(false, true)
			application, err := createSpecField(input)
			Expect(err).To(BeNil())
			Expect(application).To(Equal(getValidApplication(false)))
		})

		It("Input spec with automated enabled should set automated sync policy", func() {
			input := getfakeArgoCDSpecInput(true, false)
			application, err := createSpecField(input)
			Expect(err).To(BeNil())
			Expect(application).To(Equal(getValidApplication(true)))
		})
	})
})
