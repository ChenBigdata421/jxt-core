package crypto

import (
	"encoding/base64"
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

	Describe("Encrypt", func() {
		var service *CryptoService

		BeforeEach(func() {
			service, _ = NewCryptoService(validKey)
		})

		Context("with valid plaintext", func() {
			It("should return base64-encoded ciphertext", func() {
				plaintext := "my-secret-password"
				ciphertext, err := service.Encrypt(plaintext)

				Expect(err).ToNot(HaveOccurred())
				Expect(ciphertext).ToNot(BeEmpty())
				Expect(ciphertext).ToNot(Equal(plaintext)) // 密文应该与原文不同

				// 验证是有效的 Base64
				decoded, err := base64.StdEncoding.DecodeString(ciphertext)
				Expect(err).ToNot(HaveOccurred())
				Expect(len(decoded)).To(BeNumerically(">", 12)) // 至少包含 nonce(12) + 密文 + tag(16)
			})

			It("should produce different ciphertext each time", func() {
				plaintext := "my-secret-password"

				ciphertext1, _ := service.Encrypt(plaintext)
				ciphertext2, _ := service.Encrypt(plaintext)

				// 因为每次加密都使用随机 nonce，结果应该不同
				Expect(ciphertext1).ToNot(Equal(ciphertext2))
			})

			It("should handle empty string", func() {
				ciphertext, err := service.Encrypt("")

				Expect(err).ToNot(HaveOccurred())
				Expect(ciphertext).ToNot(BeEmpty())
			})
		})
	})

	Describe("Decrypt", func() {
		var service *CryptoService

		BeforeEach(func() {
			service, _ = NewCryptoService(validKey)
		})

		Context("with valid ciphertext", func() {
			It("should decrypt successfully", func() {
				plaintext := "my-secret-password"
				ciphertext, _ := service.Encrypt(plaintext)

				decrypted, err := service.Decrypt(ciphertext)

				Expect(err).ToNot(HaveOccurred())
				Expect(decrypted).To(Equal(plaintext))
			})

			It("should handle empty string", func() {
				ciphertext, _ := service.Encrypt("")

				decrypted, err := service.Decrypt(ciphertext)

				Expect(err).ToNot(HaveOccurred())
				Expect(decrypted).To(Equal(""))
			})
		})

		Context("with invalid ciphertext", func() {
			It("should fail with invalid base64", func() {
				_, err := service.Decrypt("not-valid-base64!!!")

				Expect(err).To(MatchError(ErrInvalidCiphertext))
			})

			It("should fail with wrong key", func() {
				wrongKey := "00000000000000000000000000000000"
				wrongService, _ := NewCryptoService(wrongKey)

				plaintext := "my-secret-password"
				ciphertext, _ := service.Encrypt(plaintext)

				_, err := wrongService.Decrypt(ciphertext)
				Expect(err).To(MatchError(ErrDecryptionFailed))
			})

			It("should fail with truncated ciphertext", func() {
				truncated := base64.StdEncoding.EncodeToString([]byte("too-short"))

				_, err := service.Decrypt(truncated)
				Expect(err).To(MatchError(ErrInvalidCiphertext))
			})
		})
	})
})

func TestCryptoService(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "CryptoService Suite")
}
