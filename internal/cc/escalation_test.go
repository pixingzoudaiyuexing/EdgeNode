package cc

import "testing"

func TestNextEscalationMultiplier(t *testing.T) {
	const now int64 = 2_000_000

	cases := []struct {
		name            string
		previous        uint32
		lastTriggeredAt int64
		want            uint32
	}{
		{name: "首次触发", previous: 0, lastTriggeredAt: 0, want: 1},
		{name: "24小时内递增", previous: 1, lastTriggeredAt: now - 60, want: 2},
		{name: "正好24小时仍连续", previous: 7, lastTriggeredAt: now - EscalationResetSeconds, want: 8},
		{name: "超过24小时重置", previous: 7, lastTriggeredAt: now - EscalationResetSeconds - 1, want: 1},
		{name: "达到32后封顶", previous: EscalationMaxMultiplier, lastTriggeredAt: now - 1, want: EscalationMaxMultiplier},
		{name: "异常大值同样封顶", previous: 99, lastTriggeredAt: now - 1, want: EscalationMaxMultiplier},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := NextEscalationMultiplier(c.previous, c.lastTriggeredAt, now); got != c.want {
				t.Fatalf("NextEscalationMultiplier() = %d, want %d", got, c.want)
			}
		})
	}
}
