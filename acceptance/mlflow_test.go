package acceptance_test

import (
	"os"
	"path"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("MLflow Model", func() {
	var org = "mlflow-org"
	BeforeEach(func() {
		out, err := Carrier("create-org "+org, "")
		Expect(err).ToNot(HaveOccurred(), out)
		out, err = Carrier("target "+org, "")
		Expect(err).ToNot(HaveOccurred(), out)
	})
	Describe("push and delete", func() {
		var appName string
		BeforeEach(func() {
			appName = "mlflow-" + strconv.Itoa(int(time.Now().Nanosecond()))
		})

		It("pushes and deletes an mlflow model", func() {
			By("pushing the mlflow model")
			currentDir, err := os.Getwd()
			Expect(err).ToNot(HaveOccurred())
			appDir := path.Join(currentDir, "../mlflow-model")

			out, err := Carrier("push --verbosity 1 "+appName, appDir)
			Expect(err).ToNot(HaveOccurred(), out)
			out, err = Carrier("apps", "")
			Expect(err).ToNot(HaveOccurred(), out)
			routeRegex := `.*\|.*1\/1.*\|.*`
			if withKnative {
				routeRegex = `.*\|.*[0,1]\/[0,1].*\|.*`
			}
			Expect(out).To(MatchRegexp(appName + routeRegex))

			By("deleting the mlflow model")
			out, err = Carrier("delete "+appName, "")
			Expect(err).ToNot(HaveOccurred(), out)
			// TODO: Fix `carrier delete` from returning before the app is deleted #131
			Eventually(func() string {
				out, err := Carrier("apps", "")
				Expect(err).ToNot(HaveOccurred(), out)
				return out
			}, "1m").ShouldNot(MatchRegexp(`.*%s.*`, appName))
		})
	})
})
