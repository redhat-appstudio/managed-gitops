package appstudioredhatcom

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Actest", func() {
	Context("It should execute all DB functions for ClusterAccess", func() {
		DescribeTable("description",
			func(appName string, suffix string, expected string) {
				res := sanitizeAppNameWithSuffix(appName, suffix)
				Expect(len(res) <= 64).To(BeTrue())
				Expect(expected).To(Equal(res))
			},
			Entry("testcase", "my-app", "", "my-app"),
			Entry("testcase", "my-app", "-deployment", "my-app-deployment"),
			Entry("testcase", "11111111111-this-is-64-chars-11111111111111111111111111111111111", "", "70b639f6884404470946307fdd5c42d9"),
			Entry("testcase", "11111111111-this-is-64-chars-11111111111111111111111111111111111", "-deployment", "70b639f6884404470946307fdd5c42d9-deployment"),
			Entry("testcase", "11111111111-this-is-53-chars-111111111111111111111111", "-deployment", "ce32f36a2818602d967afb0fce5064bd-deployment"),
			Entry("testcase", "11111111111-this-is-52-chars-11111111111111111111111", "-deployment", "11111111111-this-is-52-chars-11111111111111111111111-deployment"),
		)
	})
})
