package uam

import (
	"errors"
	"hash/fnv"

	"google.golang.org/protobuf/encoding/protowire"
)

const keyVersion int32 = 1

// Key 是 UAM 浏览器校验使用的最小兼容数据结构。
// 字段编号来自 1.3.9 Plus 的公开可观察 protobuf wire 协议：
// 1=timestamp、2=hash、3=version、4=host。
type Key struct {
	Timestamp int64
	Hash      uint64
	Version   int32
	Host      string
}

// Put 将客户端标识和 User-Agent 绑定到 Key。
// 1.3.9 使用 FNV-1a 64(a + "@" + b)。
func (k *Key) Put(a, b string) {
	k.Hash = hashPair(a, b)
}

func (k *Key) IsSame(a, b string) bool {
	return k != nil && k.Hash == hashPair(a, b)
}

func hashPair(a, b string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(a))
	_, _ = h.Write([]byte{'@'})
	_, _ = h.Write([]byte(b))
	return h.Sum64()
}

// marshal 使用 protobuf wire 格式编码，避免重新引入原私有生成文件。
func (k *Key) marshal() []byte {
	if k == nil {
		return nil
	}

	buf := make([]byte, 0, 64+len(k.Host))
	buf = protowire.AppendTag(buf, 1, protowire.VarintType)
	buf = protowire.AppendVarint(buf, uint64(k.Timestamp))
	buf = protowire.AppendTag(buf, 2, protowire.VarintType)
	buf = protowire.AppendVarint(buf, k.Hash)
	buf = protowire.AppendTag(buf, 3, protowire.VarintType)
	buf = protowire.AppendVarint(buf, uint64(k.Version))
	buf = protowire.AppendTag(buf, 4, protowire.BytesType)
	buf = protowire.AppendString(buf, k.Host)
	return buf
}

func unmarshalKey(buf []byte) (*Key, error) {
	key := &Key{}
	for len(buf) > 0 {
		num, typ, n := protowire.ConsumeTag(buf)
		if n < 0 {
			return nil, errors.New("invalid uam key tag")
		}
		buf = buf[n:]

		switch num {
		case 1:
			if typ != protowire.VarintType {
				return nil, errors.New("invalid uam key timestamp type")
			}
			v, m := protowire.ConsumeVarint(buf)
			if m < 0 {
				return nil, errors.New("invalid uam key timestamp")
			}
			key.Timestamp = int64(v)
			buf = buf[m:]
		case 2:
			if typ != protowire.VarintType {
				return nil, errors.New("invalid uam key hash type")
			}
			v, m := protowire.ConsumeVarint(buf)
			if m < 0 {
				return nil, errors.New("invalid uam key hash")
			}
			key.Hash = v
			buf = buf[m:]
		case 3:
			if typ != protowire.VarintType {
				return nil, errors.New("invalid uam key version type")
			}
			v, m := protowire.ConsumeVarint(buf)
			if m < 0 {
				return nil, errors.New("invalid uam key version")
			}
			key.Version = int32(v)
			buf = buf[m:]
		case 4:
			if typ != protowire.BytesType {
				return nil, errors.New("invalid uam key host type")
			}
			v, m := protowire.ConsumeBytes(buf)
			if m < 0 {
				return nil, errors.New("invalid uam key host")
			}
			key.Host = string(v)
			buf = buf[m:]
		default:
			m := protowire.ConsumeFieldValue(num, typ, buf)
			if m < 0 {
				return nil, errors.New("invalid uam key field")
			}
			buf = buf[m:]
		}
	}

	return key, nil
}
