package semver

import "testing"

func TestCompare(t *testing.T) {
	cases := []struct {
		left, right string
		want        int
	}{
		{"1.2.3", "1.2.3", 0},
		{"v1.2.3", "1.2.3", 0},
		{"1.10.0", "1.9.0", 1},
		{"1.2", "1.2.1", -1},
		{"1.0.0-rc.1", "1.0.0", -1},
		{"dev", "0.0.1", -1},
	}
	for _, test := range cases {
		if got := Compare(test.left, test.right); got != test.want {
			t.Errorf("compare(%q, %q) = %d, want %d", test.left, test.right, got, test.want)
		}
	}
}
