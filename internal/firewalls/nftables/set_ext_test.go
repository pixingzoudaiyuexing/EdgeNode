//go:build linux

package nftables

import (
	"testing"
	"time"

	nft "github.com/google/nftables"
)

func TestNewExpirationFromElements(t *testing.T) {
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	temporaryKey := []byte{192, 0, 2, 1}
	permanentKey := []byte{192, 0, 2, 2}

	expiration := newExpirationFromElements([]nft.SetElement{
		{
			Key:     temporaryKey,
			Expires: 90 * time.Second,
		},
		{
			Key: permanentKey,
		},
	}, now)

	temporaryExpiresAt, ok := expiration.m[string(temporaryKey)]
	if !ok {
		t.Fatal("临时元素没有写入本地索引")
	}
	if !temporaryExpiresAt.Equal(now.Add(90 * time.Second)) {
		t.Fatalf("临时元素到期时间错误: got=%s want=%s", temporaryExpiresAt, now.Add(90*time.Second))
	}

	permanentExpiresAt, ok := expiration.m[string(permanentKey)]
	if !ok {
		t.Fatal("永久元素没有写入本地索引")
	}
	if !permanentExpiresAt.IsZero() {
		t.Fatalf("永久元素不应生成到期时间: %s", permanentExpiresAt)
	}
}
