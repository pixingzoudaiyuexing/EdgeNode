package caches

import (
	"encoding/binary"
	"errors"
	"io"
	"math"

	fsutils "github.com/TeaOSLab/EdgeNode/internal/utils/fs"
	mmaputils "github.com/TeaOSLab/EdgeNode/internal/utils/mmap"
	rangeutils "github.com/TeaOSLab/EdgeNode/internal/utils/ranges"
	"github.com/iwind/TeaGo/types"
)

// MMAPFileReader 使用只读 mmap 读取完整的磁盘缓存文件。
// 文件格式和 FileReader 完全一致，只替换底层读取方式，不改变缓存协议。
type MMAPFileReader struct {
	BaseReader

	mapped *mmaputils.File
	reader *io.SectionReader

	expiresAt    int64
	status       int
	headerOffset int64
	headerSize   int
	bodySize     int64
	bodyOffset   int64

	isClosed bool
}

func NewMMAPFileReader(mapped *mmaputils.File) *MMAPFileReader {
	reader := &MMAPFileReader{mapped: mapped}
	if mapped != nil {
		reader.reader = io.NewSectionReader(mapped, 0, mapped.Size())
	}
	return reader
}

func (this *MMAPFileReader) Init() error {
	return this.InitAutoDiscard(true)
}

func (this *MMAPFileReader) InitAutoDiscard(autoDiscard bool) error {
	if this.mapped == nil || this.reader == nil {
		return ErrNotFound
	}

	var isOk bool
	if autoDiscard {
		defer func() {
			if !isOk {
				_ = this.discard()
			}
		}()
	}

	if _, err := this.reader.Seek(0, io.SeekStart); err != nil {
		return err
	}
	buf := make([]byte, SizeMeta)
	if _, err := io.ReadFull(this.reader, buf); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return ErrNotFound
		}
		return err
	}

	this.expiresAt = int64(binary.BigEndian.Uint32(buf[:SizeExpiresAt]))

	status := types.Int(string(buf[OffsetStatus : OffsetStatus+SizeStatus]))
	if status < 100 || status > 999 {
		return errors.New("invalid status")
	}
	this.status = status

	urlLength := binary.BigEndian.Uint32(buf[OffsetURLLength : OffsetURLLength+SizeURLLength])
	headerSize := int(binary.BigEndian.Uint32(buf[OffsetHeaderLength : OffsetHeaderLength+SizeHeaderLength]))
	if headerSize == 0 {
		isOk = true
		return nil
	}
	this.headerSize = headerSize
	this.headerOffset = int64(SizeMeta) + int64(urlLength)
	this.bodyOffset = this.headerOffset + int64(headerSize)

	bodySize := binary.BigEndian.Uint64(buf[OffsetBodyLength : OffsetBodyLength+SizeBodyLength])
	if bodySize > math.MaxInt64 {
		return errors.New("invalid body size")
	}
	this.bodySize = int64(bodySize)

	// mmap 路径在初始化时就校验整个缓存布局，避免越界数据被当作正常缓存继续读取。
	if this.headerOffset < int64(SizeMeta) || this.bodyOffset < this.headerOffset || this.bodyOffset > this.mapped.Size() || this.bodySize > this.mapped.Size()-this.bodyOffset {
		return ErrNotFound
	}

	isOk = true
	return nil
}

func (this *MMAPFileReader) TypeName() string {
	return "mmap"
}

func (this *MMAPFileReader) ExpiresAt() int64 {
	return this.expiresAt
}

func (this *MMAPFileReader) Status() int {
	return this.status
}

func (this *MMAPFileReader) LastModified() int64 {
	if this.mapped == nil {
		return 0
	}
	return this.mapped.ModTime().Unix()
}

func (this *MMAPFileReader) HeaderSize() int64 {
	return int64(this.headerSize)
}

func (this *MMAPFileReader) BodySize() int64 {
	return this.bodySize
}

// CopyBodyTo 直接把映射中的 Body 区域复制给目标 Writer。
// 该方法不改变当前 Reader 的游标，保持与原 Plus 具体类型接口兼容。
func (this *MMAPFileReader) CopyBodyTo(writer io.Writer) (int, error) {
	if this.mapped == nil || this.bodySize == 0 {
		return 0, nil
	}
	reader := io.NewSectionReader(this.mapped, this.bodyOffset, this.bodySize)
	n, err := io.Copy(writer, reader)
	return int(n), err
}

func (this *MMAPFileReader) ReadHeader(buf []byte, callback ReaderFunc) error {
	if this.headerSize == 0 {
		_, err := this.reader.Seek(this.bodyOffset, io.SeekStart)
		return err
	}
	if len(buf) == 0 {
		return io.ErrShortBuffer
	}

	if _, err := this.reader.Seek(this.headerOffset, io.SeekStart); err != nil {
		return err
	}

	remaining := this.headerSize
	for remaining > 0 {
		readSize := len(buf)
		if readSize > remaining {
			readSize = remaining
		}
		n, err := this.reader.Read(buf[:readSize])
		if n > 0 {
			goNext, callbackErr := callback(n)
			if callbackErr != nil {
				return callbackErr
			}
			remaining -= n
			if !goNext {
				break
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) && remaining == 0 {
				break
			}
			_ = this.discard()
			return err
		}
		if n == 0 {
			_ = this.discard()
			return io.ErrUnexpectedEOF
		}
	}

	_, err := this.reader.Seek(this.bodyOffset, io.SeekStart)
	if err != nil {
		_ = this.discard()
	}
	return err
}

func (this *MMAPFileReader) ReadBody(buf []byte, callback ReaderFunc) error {
	if this.bodySize == 0 {
		return nil
	}
	if len(buf) == 0 {
		return io.ErrShortBuffer
	}

	if _, err := this.reader.Seek(this.bodyOffset, io.SeekStart); err != nil {
		return err
	}

	remaining := this.bodySize
	for remaining > 0 {
		readSize := int64(len(buf))
		if readSize > remaining {
			readSize = remaining
		}
		n, err := this.reader.Read(buf[:int(readSize)])
		if n > 0 {
			goNext, callbackErr := callback(n)
			if callbackErr != nil {
				return callbackErr
			}
			remaining -= int64(n)
			if !goNext {
				break
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) && remaining == 0 {
				break
			}
			_ = this.discard()
			return err
		}
		if n == 0 {
			_ = this.discard()
			return io.ErrUnexpectedEOF
		}
	}
	return nil
}

func (this *MMAPFileReader) Read(buf []byte) (int, error) {
	if this.bodySize == 0 {
		return 0, io.EOF
	}
	n, err := this.reader.Read(buf)
	if err != nil && !errors.Is(err, io.EOF) {
		_ = this.discard()
	}
	return n, err
}

func (this *MMAPFileReader) ReadBodyRange(buf []byte, start int64, end int64, callback ReaderFunc) error {
	if len(buf) == 0 {
		return io.ErrShortBuffer
	}

	offset := start
	if start < 0 {
		offset = this.bodyOffset + this.bodySize + end
		end = this.bodyOffset + this.bodySize - 1
	} else if end < 0 {
		offset = this.bodyOffset + start
		end = this.bodyOffset + this.bodySize - 1
	} else {
		offset = this.bodyOffset + start
		end = this.bodyOffset + end
	}
	if offset < 0 || end < 0 || offset > end {
		return ErrInvalidRange
	}
	if _, err := this.reader.Seek(offset, io.SeekStart); err != nil {
		return err
	}

	for offset <= end {
		readSize := int64(len(buf))
		remaining := end - offset + 1
		if readSize > remaining {
			readSize = remaining
		}
		n, err := this.reader.Read(buf[:int(readSize)])
		if n > 0 {
			goNext, callbackErr := callback(n)
			if callbackErr != nil {
				return callbackErr
			}
			offset += int64(n)
			if !goNext {
				break
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) && offset > end {
				break
			}
			_ = this.discard()
			return err
		}
		if n == 0 {
			_ = this.discard()
			return io.ErrUnexpectedEOF
		}
	}

	if this.nextReader != nil {
		defer func() { _ = this.nextReader.Close() }()
		for {
			n, err := this.nextReader.Read(buf)
			if n > 0 {
				goNext, callbackErr := callback(n)
				if callbackErr != nil {
					return callbackErr
				}
				if !goNext {
					break
				}
			}
			if err != nil {
				if !errors.Is(err, io.EOF) {
					return err
				}
				break
			}
		}
	}

	return nil
}

func (this *MMAPFileReader) ContainsRange(r rangeutils.Range) (rangeutils.Range, bool) {
	return r, true
}

func (this *MMAPFileReader) Close() error {
	if this.isClosed {
		return nil
	}
	this.isClosed = true
	if this.mapped == nil {
		return nil
	}
	return this.mapped.Close()
}

func (this *MMAPFileReader) discard() error {
	if this.isClosed {
		return nil
	}
	this.isClosed = true
	if this.mapped == nil {
		return nil
	}

	name := this.mapped.Name()
	isCurrent := this.mapped.IsCurrent()
	_ = this.mapped.Close()
	if !isCurrent || len(name) == 0 {
		return nil
	}
	return fsutils.Remove(name)
}
