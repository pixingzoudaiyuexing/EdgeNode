package cc

import "testing"

func TestReachedMinQPS(t *testing.T) {
	cases := []struct {
		name     string
		minQPS   int
		requests int64
		want     bool
	}{
		{name: "零门槛", minQPS: 0, requests: 0, want: true},
		{name: "负门槛兼容为未设置", minQPS: -1, requests: 0, want: true},
		{name: "刚好达到", minQPS: 2, requests: 120, want: true},
		{name: "超过门槛", minQPS: 2, requests: 121, want: true},
		{name: "低于门槛", minQPS: 2, requests: 119, want: false},
		{name: "非法负请求数", minQPS: 2, requests: -1, want: false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ReachedMinQPS(c.minQPS, c.requests); got != c.want {
				t.Fatalf("ReachedMinQPS(%d, %d) = %v, want %v", c.minQPS, c.requests, got, c.want)
			}
		})
	}
}
