package version

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.2.3", "1.2.3", 0},
		{"1.2.3", "1.2.4", -1},
		{"1.10.0", "1.9.0", 1},
		{"2.0.0", "1.99.99", 1},
		{"v1.2.3", "1.2.3", 0},
		{"1.2.3+build.5", "1.2.3", 0},
		{"1.0.0-alpha", "1.0.0", -1},
		{"1.0.0", "1.0.0-rc.1", 1},
		{"1.0.0-alpha", "1.0.0-alpha.1", -1},
		{"1.0.0-alpha.1", "1.0.0-alpha.beta", -1},
		{"1.0.0-alpha.beta", "1.0.0-beta", -1},
		{"1.0.0-beta.2", "1.0.0-beta.11", -1},
		{"1.0.0-rc.1", "1.0.0-beta.11", 1},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, Compare(c.a, c.b), "%s against %s", c.a, c.b)
		assert.Equal(t, -c.want, Compare(c.b, c.a), "%s against %s", c.b, c.a)
	}
}

func TestCompare_DevOrUnreadableIsTheSame(t *testing.T) {
	for _, other := range []string{"dev", "", "1.2", "1.2.3.4", "01.2.3", "1.2.x", "1.2.3-", "1.2.3-a..b"} {
		assert.Zero(t, Compare(other, "9.9.9"), "%q", other)
		assert.Zero(t, Compare("0.0.1", other), "%q", other)
	}
}
