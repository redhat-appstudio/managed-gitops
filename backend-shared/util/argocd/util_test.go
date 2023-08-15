package argocd

import (
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Test Argo CD utility functions", func() {

	Context("Test ConvertArgoCDClusterSecretNameToManagedIdDatabaseRowId", func() {

		When("the cluster secret name starts with the managed env prefix", func() {

			It("should return the value for after the managed env prefix", func() {
				name, isLocal, err := ConvertArgoCDClusterSecretNameToManagedIdDatabaseRowId(managedEnvPrefix + "1234")
				Expect(name).To(Equal("1234"))
				Expect(err).ToNot(HaveOccurred())
				Expect(isLocal).To(BeFalse())
			})

			It("should return another value for after the managed env prefix", func() {
				name, isLocal, err := ConvertArgoCDClusterSecretNameToManagedIdDatabaseRowId(managedEnvPrefix + "1234-5678-910112")
				Expect(name).To(Equal("1234-5678-910112"))
				Expect(err).ToNot(HaveOccurred())
				Expect(isLocal).To(BeFalse())
			})

		})

		When("the cluster secret name is local", func() {
			It("should return isLocal to be true", func() {
				name, isLocal, err := ConvertArgoCDClusterSecretNameToManagedIdDatabaseRowId(ArgoCDDefaultDestinationInCluster)
				Expect(name).To(Equal(""))
				Expect(err).ToNot(HaveOccurred())
				Expect(isLocal).To(BeTrue())
			})
		})

		When("the cluster secret name does not start with managed env prefix, and is not a local secret", func() {

			It("should should return an error", func() {
				name, isLocal, err := ConvertArgoCDClusterSecretNameToManagedIdDatabaseRowId("just-a-random-secret-name")
				Expect(name).To(Equal(""))
				Expect(err).To(HaveOccurred())
				Expect(isLocal).To(BeFalse())

			})
		})
	})

	Context("Test GetArgoCDApplicationName", func() {
		When("the Argo CD resource label is found and it has a gitopdepl prefix", func() {
			It("should return the name of the Application", func() {
				expectedName := "gitopsdepl-sample"
				labels := map[string]string{
					ArgocdResourceLabel: expectedName,
				}
				name := GetArgoCDApplicationName(labels)
				Expect(name).To(Equal(expectedName))
			})
		})

		When("the Argo CD resource label is found but it doesn't have a gitopdepl prefix", func() {
			It("should return an empty string", func() {
				labels := map[string]string{
					ArgocdResourceLabel: "invalid-name",
				}
				name := GetArgoCDApplicationName(labels)
				Expect(name).To(BeEmpty())
			})
		})

		When("the Argo CD resource label is not found", func() {
			It("should return an empty string", func() {
				labels := map[string]string{}
				name := GetArgoCDApplicationName(labels)
				Expect(name).To(BeEmpty())
			})
		})
	})

	Context("Test ExtractUIDFromApplicationName", func() {
		When("an invalid Application name is passed", func() {
			It("should return an empty string", func() {
				name := ExtractUIDFromApplicationName("invalidappname")
				Expect(name).To(BeEmpty())
			})
		})

		When("the Application name has an invalid gitopsdepl prefix", func() {
			It("should return an empty string", func() {
				name := ExtractUIDFromApplicationName("sample-gitopsdepl-invalidappname")
				Expect(name).To(BeEmpty())
			})
		})

		When("a valid Application name is passed", func() {
			It("should return the UID from the name", func() {
				expectedUID := uuid.New().String()
				name := ExtractUIDFromApplicationName("gitopsdepl-" + expectedUID)
				Expect(name).To(Equal(expectedUID))
			})
		})
	})
})
