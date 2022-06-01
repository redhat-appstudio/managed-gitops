package db

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_isEnvExist(t *testing.T) {
	type args struct {
		envVar string
		err    error
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{name: "Env variable exists", args: args{envVar: "FOO", err: os.Setenv("FOO", "bar")}, want: true},
		{name: "Env variable is case sensitive", args: args{envVar: "foo", err: os.Setenv("FOO", "bar")}, want: false},
		{name: "Env variable does not exist", args: args{envVar: "doesNotExist", err: nil}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, IsEnvExist(tt.args.envVar), "IsEnvExist(%v)", tt.args.envVar)
		})
	}
}
