package caches

import (
	"errors"
	"os"

	mmaputils "github.com/TeaOSLab/EdgeNode/internal/utils/mmap"
)

// tryMMAPReader 尝试使用 mmap 读取完整磁盘缓存。
// mmap 是可选优化：平台不支持或映射资源不足时回退普通文件 I/O，不影响缓存可用性。
func (this *FileStorage) tryMMAPReader(isPartial bool, estimatedSize int64, path string) (Reader, error) {
	if this.options == nil || !this.options.EnableMMAP || isPartial || estimatedSize <= 0 {
		return nil, nil
	}

	mapped, err := mmaputils.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		if errors.Is(err, mmaputils.ErrUnsupported) || errors.Is(err, mmaputils.ErrEmptyFile) || errors.Is(err, mmaputils.ErrFileTooLarge) {
			return nil, nil
		}
		// mmap 只是读取优化，ENOMEM、地址空间不足等错误不应让原本可读的缓存失效。
		return nil, nil
	}

	reader := NewMMAPFileReader(mapped)
	if err = reader.Init(); err != nil {
		return nil, err
	}
	return reader, nil
}
