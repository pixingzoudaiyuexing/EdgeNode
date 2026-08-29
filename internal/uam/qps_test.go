package uam

import "testing"

func TestQPSTrackerOneMinuteAverage(t *testing.T) {
	tracker := NewQPSTracker()
	const key = "server-1@203.0.113.20"
	const minQPS = 2
	const start int64 = 1000

	for second := int64(0); second < 60; second++ {
		for request := 0; request < minQPS; request++ {
			triggered := tracker.triggeredAt(key, minQPS, start+second)
			if second == 59 && request == minQPS-1 {
				if !triggered {
					t.Fatal("expected one-minute average QPS to trigger")
				}
			} else if triggered {
				t.Fatalf("triggered too early at second=%d request=%d", second, request)
			}
		}
	}
}

func TestQPSTrackerExpiresOldBuckets(t *testing.T) {
	tracker := NewQPSTracker()
	const key = "server-1@203.0.113.21"

	for i := 0; i < 60; i++ {
		if triggered := tracker.triggeredAt(key, 1, 1000); triggered && i < 59 {
			t.Fatalf("triggered too early at request=%d", i)
		}
	}
	if !tracker.triggeredAt(key, 1, 1000) {
		t.Fatal("expected threshold to be reached")
	}

	if tracker.triggeredAt(key, 1, 1061) {
		t.Fatal("requests older than 60 seconds should not keep threshold active")
	}
}

func TestQPSTrackerZeroThresholdAlwaysTriggers(t *testing.T) {
	tracker := NewQPSTracker()
	if !tracker.triggeredAt("server-1@203.0.113.22", 0, 1000) {
		t.Fatal("zero min QPS should trigger immediately")
	}
}
