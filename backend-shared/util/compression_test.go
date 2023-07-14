package util

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v2"
)

var _ = Describe("Verify compress and decompress functions", func() {
	Context("Compress and decompress sample data", func() {
		It("should verify that the object can be compressed and decompressed successfully", func() {
			By("compress the sample object")
			type sampleObject struct {
				Name      string
				Namespace string
			}

			obj := sampleObject{
				Name:      "sample",
				Namespace: "test",
			}

			compressedBytes, err := CompressObject(obj)
			Expect(err).To(BeNil())
			Expect(compressedBytes).ToNot(BeNil())

			By("decompress the sample object and verify the data")

			decompressedBytes, err := DecompressObject(compressedBytes)
			Expect(err).To(BeNil())
			Expect(decompressedBytes).ToNot(BeNil())

			decompressedObj := &sampleObject{}
			err = yaml.Unmarshal(decompressedBytes, decompressedObj)
			Expect(err).To(BeNil())

			Expect(decompressedObj.Name).To(Equal(obj.Name))
			Expect(decompressedObj.Namespace).To(Equal(obj.Namespace))
		})

		It("should not panic when nil is passed", func() {
			compressedBytes, err := CompressObject(nil)
			Expect(err).To(BeNil())
			Expect(compressedBytes).To(BeNil())

			decompressedBytes, err := DecompressObject(nil)
			Expect(err).To(BeNil())
			Expect(decompressedBytes).To(BeNil())
		})
	})
})
