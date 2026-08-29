package cc

import "testing"

func TestThresholdReachedAtEquality(t *testing.T) {
	if ThresholdReached(29, 30) {
		t.Fatal("29/30 不应触发")
	}
	if !ThresholdReached(30, 30) {
		t.Fatal("30/30 应在等于阈值时立即触发")
	}
	if !ThresholdReached(31, 30) {
		t.Fatal("超过阈值应触发")
	}
	if ThresholdReached(1, 0) {
		t.Fatal("MaxRequests<=0 不应触发")
	}
}
