package uslice

import (
	"reflect"
	"testing"
)

func TestCompressIntRanges(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{name: "empty", in: nil, want: nil},
		{name: "single", in: []string{"5"}, want: []string{"5"}},
		{name: "contiguous", in: []string{"1", "2", "3"}, want: []string{"1-3"}},
		{name: "mixed", in: []string{"1", "2", "3", "5", "8", "9"}, want: []string{"1-3", "5", "8-9"}},
		{name: "duplicates", in: []string{"1", "1", "2", "2", "3"}, want: []string{"1-3"}},
		{name: "non_numeric", in: []string{"a", "b"}, want: []string{"a", "b"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CompressIntRanges(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("CompressIntRanges(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}
