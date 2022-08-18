package db

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestIsEmptyValues(t *testing.T) {

	for _, c := range []struct {
		// name is human-readable test name
		name          string
		params        []any
		expectedError bool
	}{
		{
			name:          "valid",
			params:        []any{"key", "value"},
			expectedError: false,
		},
		{
			name:          "empty key w/ string value",
			params:        []any{"", "value"},
			expectedError: true,
		},
		{
			name:          "empty value w/ string key",
			params:        []any{"key", ""},
			expectedError: true,
		},
		{
			name:          "empty string value, w/ string key",
			params:        []any{"key", ""},
			expectedError: true,
		},
		{
			name:          "empty interface value, w/ string key",
			params:        []any{"key", nil},
			expectedError: true,
		},
		{
			name:          "empty interface key, w/ string value",
			params:        []any{nil, ""},
			expectedError: true,
		},
		{
			name:          "multiple valid string values",
			params:        []any{"key", "value", "key2", "value2"},
			expectedError: false,
		},
		{
			name:          "multiple string values, one invalid",
			params:        []any{"key", "value", "key2", ""},
			expectedError: true,
		},
		{
			name:          "multiple mixed values, one invalid",
			params:        []any{"key", nil, "key2", ""},
			expectedError: true,
		},
		{
			name:          "multiple mixed values, one invalid, different order",
			params:        []any{"key", "hi", "key2", nil},
			expectedError: true,
		},

		{
			name:          "invalid number of params",
			params:        []any{"key", "value", "key2"},
			expectedError: true,
		},
		{
			name:          "no params",
			params:        []any{},
			expectedError: true,
		},
	} {
		t.Run(c.name, func(t *testing.T) {

			res := isEmptyValues(c.name, c.params...)
			assert.True(t, (res != nil) == c.expectedError)

		})
	}
}

var _ = Describe("Test utility functions", func() {
	Context("Ensure ValidateFieldLength reports database rows that are too long", func() {
		It("Should return error for Clusteruser_id", func() {
			user := &ClusterUser{
				Clusteruser_id: strings.Repeat("abc", 17),
				User_name:      "user-name",
			}
			err := validateFieldLength(user)
			Expect(IsMaxLengthError(err)).To(BeTrue())
		})

		It("Should return error for User_name", func() {
			user := &ClusterUser{
				Clusteruser_id: "user-id",
				User_name:      strings.Repeat("abc", 86),
			}
			err := validateFieldLength(user)
			Expect(IsMaxLengthError(err)).To(BeTrue())
		})

		It("Should pass", func() {
			user := &ClusterUser{
				Clusteruser_id: "user-id",
				User_name:      "user-name",
			}
			err := validateFieldLength(user)
			Expect(IsMaxLengthError(err)).To(BeFalse())
		})

		It("Should pass.", func() {
			gitopsEngineCluster := &GitopsEngineCluster{
				Gitopsenginecluster_id: "user-id",
				SeqID:                  123,
			}
			err := validateFieldLength(gitopsEngineCluster)
			Expect(IsMaxLengthError(err)).To(BeFalse())
		})

		It("Should return error for Gitopsenginecluster_id", func() {
			gitopsEngineCluster := &GitopsEngineCluster{
				Gitopsenginecluster_id: strings.Repeat("abc", 17),
				SeqID:                  123,
			}
			err := validateFieldLength(gitopsEngineCluster)
			Expect(IsMaxLengthError(err)).To(BeTrue())
		})
	})

	Context("Ensure IsMaxLengthError checks for error returned by validateFieldLength.", func() {
		It("Should return True", func() {
			Expect(IsMaxLengthError(fmt.Errorf("%v value exceeds maximum size: max: %d, actual: %d", "fieldName", 5, 10))).To(BeTrue())
		})

		It("Should return False", func() {
			Expect(IsMaxLengthError(fmt.Errorf("Some error"))).To(BeFalse())
		})

		It("Should return False", func() {
			Expect(IsMaxLengthError(nil)).To(BeFalse())
		})
	})
})
