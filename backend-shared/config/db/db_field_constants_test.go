package db

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTruncateVarchar(t *testing.T) {
	type args struct {
		s         string
		maxLength int
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "String Larger Than Max",
			args: args{
				s:         "1234567890",
				maxLength: 8,
			},
			want: "12345...",
		},
		{
			name: "String Smaller Than Max",
			args: args{
				s:         "1234567890",
				maxLength: 11,
			},
			want: "1234567890",
		},
		{
			name: "String Equals Max",
			args: args{
				s:         "1234567890",
				maxLength: 10,
			},
			want: "1234567890",
		},
		{
			name: "Runes",
			args: args{
				s:         "αβγδεζηθικλμνξοπρστω",
				maxLength: 5,
			},
			want: "αβ...",
		},
		{
			name: "Runes with spaces",
			args: args{
				s:         "αβ γδ εζ η θικλ μν ξο  πρ σ τω",
				maxLength: 5,
			},
			want: "αβ...",
		},
		{
			name: "Multi-byte string",
			args: args{
				s:         "僤凘墈 葎萻萶...",
				maxLength: 7,
			},
			want: "僤凘墈 ...",
		},
		{
			name: "MaxLength 0",
			args: args{
				s:         "僤凘墈 葎萻萶...",
				maxLength: 0,
			},
			want: "",
		},
		{
			name: "MaxLength 3",
			args: args{
				s:         "僤凘墈 葎萻萶...",
				maxLength: 3,
			},
			want: "...",
		},
		{
			name: "MaxLength 2",
			args: args{
				s:         "僤凘墈 葎萻萶...",
				maxLength: 2,
			},
			want: "..",
		},
		{
			name: "MaxLength 1",
			args: args{
				s:         "僤凘墈 葎萻萶...",
				maxLength: 1,
			},
			want: ".",
		},
		{
			name: "MaxLength -1",
			args: args{
				s:         "僤凘墈 葎萻萶...",
				maxLength: -1,
			},
			want: "",
		},
		{
			name: "Emoji",
			args: args{
				s:         "😈👻👾🤖🎃⛑",
				maxLength: 5,
			},
			want: "😈👻...",
		},
		{
			name: "Invalid UTF-8 rune",
			args: args{
				s:         string([]byte{0xff, 0xfe, 0xfd}),
				maxLength: 5,
			},
			want: "",
		},
		{
			name: "Empty string",
			args: args{
				s:         "",
				maxLength: 5,
			},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, TruncateVarchar(tt.args.s, tt.args.maxLength), "TruncateVarchar(%v, %v)", tt.args.s, tt.args.maxLength)
		})
	}
}

func TestGetConstantValue(t *testing.T) {
	for field, value := range DbFieldMap {
		result := getConstantValue(field)
		assert.Equal(t, value, result)
	}
}
