// Copyright 2021 GoEdge goedge.cdn@gmail.com. All rights reserved.

package compressions

import (
	"github.com/andybalholm/brotli"
	"io"
)

// BrotliWriter 使用公开的纯 Go Brotli 实现。
// 自维护版本在 Linux + plus 构建下也使用该实现，保持原有 Writer/Pool 接口并移除对私有 C-Brotli 的依赖。
type BrotliWriter struct {
	BaseWriter

	writer *brotli.Writer
	level  int
}

func NewBrotliWriter(writer io.Writer, level int) (Writer, error) {
	return sharedBrotliWriterPool.Get(writer, level)
}

func newBrotliWriter(writer io.Writer) (*BrotliWriter, error) {
	var level = GenerateCompressLevel(brotli.BestSpeed, brotli.BestCompression)
	return &BrotliWriter{
		writer: brotli.NewWriterOptions(writer, brotli.WriterOptions{
			Quality: level,
			LGWin:   14, // TODO 在全局设置里可以设置此值
		}),
		level: level,
	}, nil
}

func (this *BrotliWriter) Write(p []byte) (int, error) {
	return this.writer.Write(p)
}

func (this *BrotliWriter) Flush() error {
	return this.writer.Flush()
}

func (this *BrotliWriter) Reset(newWriter io.Writer) {
	this.writer.Reset(newWriter)
}

func (this *BrotliWriter) RawClose() error {
	return this.writer.Close()
}

func (this *BrotliWriter) Close() error {
	return this.Finish(this)
}

func (this *BrotliWriter) Level() int {
	return this.level
}
