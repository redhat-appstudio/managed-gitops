package application_event_loop

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Application Event Runner Deployments", func() {
	Context("createSpecField() should generate a valid argocd Application", func() {
		It("Input spec is converted to an argocd Application", func() {
			input := fakeInputSpec(false, false)
			application, err := createSpecField(input)
			Expect(err).To(BeNil())
			Expect(application).To(Equal(validApplication()))
		})

		It("Sanitize illegal characters from input", func() {
			input := fakeInputSpec(false, true)
			application, err := createSpecField(input)
			Expect(err).To(BeNil())
			Expect(application).To(Equal(validApplication()))
		})

		It("Input spec with automated enabled should set automated sync policy", func() {
			input := fakeInputSpec(true, false)
			application, err := createSpecField(input)
			Expect(err).To(BeNil())
			Expect(application).To(Equal(validApplicationWithAutomatedSync()))
		})
	})
})

func fakeInputSpec(automated, unsanitized bool) argoCDSpecInput {
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

func validApplicationWithAutomatedSync() string {
	return `fauxtypemeta:
  kind: Application
  apiversion: argoproj.io/v1alpha1
fauxobjectmeta:
  name: sample-depl
  namespace: kcp-workspace
spec:
  source:
    repourl: https://github.com/test/test
    path: environments/prod
    targetrevision: ""
  destination:
    server: ""
    namespace: prod
    name: in-cluster
  project: default
  syncpolicy:
    automated:
      prune: true
      selfheal: true
      allowempty: true
    syncoptions: []
    retry: null
`
}

func validApplication() string {
	return `fauxtypemeta:
  kind: Application
  apiversion: argoproj.io/v1alpha1
fauxobjectmeta:
  name: sample-depl
  namespace: kcp-workspace
spec:
  source:
    repourl: https://github.com/test/test
    path: environments/prod
    targetrevision: ""
  destination:
    server: ""
    namespace: prod
    name: in-cluster
  project: default
  syncpolicy: null
`
}
