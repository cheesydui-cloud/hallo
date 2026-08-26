package xray

import "testing"

func TestParseLimitBytes(t *testing.T) {
	cases := map[string]int64{
		"0":   0,
		"10g": 10 << 30,
		"100m": 100 << 20,
		"12":  12,
	}
	for in, want := range cases {
		got, err := ParseLimitBytes(in)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("%s: got %d want %d", in, got, want)
		}
	}
}
