package db

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsEmptyValues(t *testing.T) {

	for _, c := range []struct {
		// name is human-readable test name
		name          string
		params        []interface{}
		expectedError bool
	}{
		{
			name:          "valid",
			params:        []interface{}{"key", "value"},
			expectedError: false,
		},
		{
			name:          "empty key w/ string value",
			params:        []interface{}{"", "value"},
			expectedError: true,
		},
		{
			name:          "empty value w/ string key",
			params:        []interface{}{"key", ""},
			expectedError: true,
		},
		{
			name:          "empty string value, w/ string key",
			params:        []interface{}{"key", ""},
			expectedError: true,
		},
		{
			name:          "empty interface value, w/ string key",
			params:        []interface{}{"key", nil},
			expectedError: true,
		},
		{
			name:          "empty interface key, w/ string value",
			params:        []interface{}{nil, ""},
			expectedError: true,
		},
		{
			name:          "multiple valid string values",
			params:        []interface{}{"key", "value", "key2", "value2"},
			expectedError: false,
		},
		{
			name:          "multiple string values, one invalid",
			params:        []interface{}{"key", "value", "key2", ""},
			expectedError: true,
		},
		{
			name:          "multiple mixed values, one invalid",
			params:        []interface{}{"key", nil, "key2", ""},
			expectedError: true,
		},
		{
			name:          "multiple mixed values, one invalid, different order",
			params:        []interface{}{"key", "hi", "key2", nil},
			expectedError: true,
		},

		{
			name:          "invalid number of params",
			params:        []interface{}{"key", "value", "key2"},
			expectedError: true,
		},
		{
			name:          "no params",
			params:        []interface{}{},
			expectedError: true,
		},
	} {
		t.Run(c.name, func(t *testing.T) {

			res := isEmptyValues(c.name, c.params...)
			assert.True(t, (res != nil) == c.expectedError)

		})
	}
}
