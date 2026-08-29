//go:build linux

package nftables

import (
	"time"

	nft "github.com/google/nftables"
)

// initElements 在 Set 创建时同步内核里已经存在的元素。
// 这层索引只用于加速重复添加和删除判断，不替代内核 nftables 的真实状态。
func (this *Set) initElements() {
	elements, err := this.conn.Raw().GetSetElements(this.rawSet)
	if err != nil {
		return
	}

	this.expiration = newExpirationFromElements(elements, time.Now())
}

// newExpirationFromElements 将内核返回的剩余有效期转换为本地绝对到期时间。
// Expires 为零时表示该元素没有可用的到期时间，本地索引按长期存在处理。
func newExpirationFromElements(elements []nft.SetElement, now time.Time) *Expiration {
	expiration := NewExpiration()
	for _, element := range elements {
		expiresAt := time.Time{}
		if element.Expires != 0 {
			expiresAt = now.Add(element.Expires)
		}
		expiration.AddUnsafe(element.Key, expiresAt)
	}
	return expiration
}
