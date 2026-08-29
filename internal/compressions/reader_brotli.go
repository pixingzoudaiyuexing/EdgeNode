// Copyright 2021 GoEdge goedge.cdn@gmail.com. All rights reserved.

package compressions

import (
	"github.com/andybalholm/brotli"
	"io"
	"strings"
)

// BrotliReader 使用公开的纯 Go Brotli 实现。
// 自维护版本在 Linux + plus 构建下也使用该实现，避免依赖原 Plus 私有的 C-Brotli 代码和额外系统库。
type BrotliReader struct {
	BaseReader

	reader *brotli.Reader
}

func NewBrotliReader(reader io.Reader) (Reader, error) {
	return sharedBrotliReaderPool.Get(reader)
}

func newBrotliReader(reader io.Reader) (Reader, error) {
	return &BrotliReader{reader: brotli.NewReader(reader)}, nil
}

func (this *BrotliReader) Read(p []byte) (n int, err error) {
	n, err = this.reader.Read(p)
	if err != nil && strings.Contains(err.Error(), "excessive") {
		err = io.EOF
	}
	return
}

func (this *BrotliReader) Reset(reader io.Reader) error {
	return this.reader.Reset(reader)
}

func (this *BrotliReader) RawClose() error {
	return nil
}

func (this *BrotliReader) Close() error {
	return this.Finish(this)
}
