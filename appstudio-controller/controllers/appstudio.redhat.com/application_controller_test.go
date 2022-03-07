package appstudioredhatcom

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSanitizeAppNameWithSuffix(t *testing.T) {

	testVals := []struct {
		appName  string
		suffix   string
		expected string
	}{
		{appName: "my-app", suffix: "", expected: "my-app"},
		{appName: "my-app", suffix: "-deployment", expected: "my-app-deployment"},
		{appName: "11111111111-this-is-64-chars-11111111111111111111111111111111111", suffix: "", expected: "70b639f6884404470946307fdd5c42d9"},
		{appName: "11111111111-this-is-64-chars-11111111111111111111111111111111111", suffix: "-deployment", expected: "70b639f6884404470946307fdd5c42d9-deployment"},
		{appName: "11111111111-this-is-53-chars-111111111111111111111111", suffix: "-deployment", expected: "ce32f36a2818602d967afb0fce5064bd-deployment"},
		{appName: "11111111111-this-is-52-chars-11111111111111111111111", suffix: "-deployment", expected: "11111111111-this-is-52-chars-11111111111111111111111-deployment"},
	}

	for _, testVal := range testVals {

		t.Run("testing"+testVal.appName+"-"+testVal.suffix, func(t *testing.T) {

			res := sanitizeAppNameWithSuffix(testVal.appName, testVal.suffix)

			assert.True(t, len(res) <= 64)

			assert.Equal(t, testVal.expected, res)

		})

	}

}
