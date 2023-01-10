package db

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Db Field Constants Test", func() {
	Context("Test truncate varchar", func() {
		DescribeTable("description",
			func(expected string, s string, maxLength int) {
				Expect(expected).To(Equal(TruncateVarchar(s, maxLength)))
			},
			Entry("String Larger Than Max", "12345...", "1234567890", 8),
			Entry("String Smaller Than Max", "1234567890", "1234567890", 11),
			Entry("String Equals Max", "1234567890", "1234567890", 10),
			Entry("Runes", "αβ...", "αβγδεζηθικλμνξοπρστω", 5),
			Entry("Runes with spaces", "αβ...", "αβ γδ εζ η θικλ μν ξο  πρ σ τω", 5),
			Entry("Multi-byte string", "僤凘墈 ...", "僤凘墈 葎萻萶...", 7),
			Entry("MaxLength 0", "", "僤凘墈 葎萻萶...", 0),
			Entry("MaxLength 3", "...", "僤凘墈 葎萻萶...", 3),
			Entry("MaxLength 2", "..", "僤凘墈 葎萻萶...", 2),
			Entry("MaxLength 1", ".", "僤凘墈 葎萻萶...", 1),
			Entry("MaxLength -1", "", "僤凘墈 葎萻萶...", -1),
			Entry("Emoji", "😈👻...", "😈👻👾🤖🎃⛑", 5),
			Entry("Invalid UTF-8 rune", "", string([]byte{0xff, 0xfe, 0xfd}), 5),
			Entry("Empty string", "", "", 5),
		)
	})
	Context("Test getConstantValue", func() {
		for field, value := range DbFieldMap {
			result := getConstantValue(field)
			Expect(value).To(Equal(result))
		}
	})
})
