package crypto

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("CryptoService", func() {
	var (
		validKey = "12345678901234567890123456789012" // 32 bytes
		shortKey = "short"
	)

	Describe("NewCryptoService", func() {
		Context("with valid 32-byte key", func() {
			It("should create service successfully", func() {
				service, err := NewCryptoService(validKey)
				Expect(err).ToNot(HaveOccurred())
				Expect(service).ToNot(BeNil())
			})
		})

		Context("with invalid key length", func() {
			It("should return ErrInvalidKeyLength", func() {
				_, err := NewCryptoService(shortKey)
				Expect(err).To(MatchError(ErrInvalidKeyLength))
			})
		})
	})
})

func TestCryptoService(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "CryptoService Suite")
}
