package acceptance_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Orgs", func() {
	It("has a default org", func() {
		orgs, err := Fuseml("orgs", "")
		Expect(err).ToNot(HaveOccurred())
		Expect(orgs).To(MatchRegexp("workspace"))
	})

	Describe("create-org", func() {
		It("creates and targets an org", func() {
			By("creating an org")
			out, err := Fuseml("create-org mycreatedorg", "")
			Expect(err).ToNot(HaveOccurred(), out)
			orgs, err := Fuseml("orgs", "")
			Expect(err).ToNot(HaveOccurred())
			Expect(orgs).To(MatchRegexp("mycreatedorg"))

			By("targeting an org")
			out, err = Fuseml("target mycreatedorg", "")
			Expect(err).ToNot(HaveOccurred(), out)
			out, err = Fuseml("target", "")
			Expect(err).ToNot(HaveOccurred(), out)
			Expect(out).To(MatchRegexp("Currently targeted organization: mycreatedorg"))

			By("switching org back to default")
			out, err = Fuseml("target workspace", "")
			Expect(err).ToNot(HaveOccurred(), out)
		})
	})
})
