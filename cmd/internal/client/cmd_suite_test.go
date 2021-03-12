package client_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestFuseml(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Fuseml Suite")
}
