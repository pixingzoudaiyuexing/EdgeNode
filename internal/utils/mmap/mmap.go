package mmap

import (
	"errors"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrUnsupported  = errors.New("mmap is unsupported on this platform")
	ErrEmptyFile    = errors.New("can not mmap an empty file")
	ErrFileTooLarge = errors.New("file is too large to mmap")
)

type fileIdentity struct {
	size    int64
	modTime int64
	device  uint64
	inode   uint64
}

type sharedMapping struct {
	path     string
	data     []byte
	identity fileIdentity
	info     os.FileInfo
	refs     int
	closed   bool
}

// File 是一个只读共享内存映射句柄。同一路径、同一文件版本的并发读取会共享映射，
// 最后一个引用关闭后才执行 Munmap。
type File struct {
	mapping *sharedMapping
	closed  atomic.Bool
}

var registryLocker sync.Mutex
var registry = map[string]*sharedMapping{}

// Open 打开一个只读内存映射。文件被原子替换后，新请求会创建新映射，
// 已持有旧映射的请求仍可安全完成读取。
func Open(path string) (*File, error) {
	fp, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer fp.Close()

	info, err := fp.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() <= 0 {
		return nil, ErrEmptyFile
	}

	device, inode := platformFileIdentity(info)
	identity := fileIdentity{
		size:    info.Size(),
		modTime: info.ModTime().UnixNano(),
		device:  device,
		inode:   inode,
	}

	registryLocker.Lock()
	if current := registry[path]; current != nil && !current.closed && current.identity == identity {
		current.refs++
		registryLocker.Unlock()
		return &File{mapping: current}, nil
	}

	data, err := mapOpenedFile(fp, info.Size())
	if err != nil {
		registryLocker.Unlock()
		return nil, err
	}

	mapping := &sharedMapping{
		path:     path,
		data:     data,
		identity: identity,
		info:     info,
		refs:     1,
	}
	registry[path] = mapping
	registryLocker.Unlock()

	return &File{mapping: mapping}, nil
}

// ReadAt 实现 io.ReaderAt，供缓存 Reader 使用 SectionReader 做顺序和随机读取。
func (this *File) ReadAt(p []byte, off int64) (int, error) {
	if this == nil || this.mapping == nil || this.closed.Load() {
		return 0, os.ErrClosed
	}
	if off < 0 {
		return 0, errors.New("negative mmap offset")
	}
	if off >= int64(len(this.mapping.data)) {
		return 0, io.EOF
	}

	n := copy(p, this.mapping.data[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func (this *File) Size() int64 {
	if this == nil || this.mapping == nil {
		return 0
	}
	return int64(len(this.mapping.data))
}

func (this *File) Name() string {
	if this == nil || this.mapping == nil {
		return ""
	}
	return this.mapping.path
}

func (this *File) ModTime() time.Time {
	if this == nil || this.mapping == nil || this.mapping.info == nil {
		return time.Time{}
	}
	return this.mapping.info.ModTime()
}

func (this *File) Stat() os.FileInfo {
	if this == nil || this.mapping == nil {
		return nil
	}
	return this.mapping.info
}

// IsCurrent 检查当前路径是否仍指向创建映射时的同一个文件。
// 缓存文件采用原子替换时，可避免旧 Reader 在报错清理时误删新文件。
func (this *File) IsCurrent() bool {
	if this == nil || this.mapping == nil {
		return false
	}
	info, err := os.Stat(this.mapping.path)
	if err != nil {
		return false
	}
	device, inode := platformFileIdentity(info)
	identity := fileIdentity{
		size:    info.Size(),
		modTime: info.ModTime().UnixNano(),
		device:  device,
		inode:   inode,
	}
	return identity == this.mapping.identity
}

// Close 释放当前引用；最后一个引用负责解除映射。
func (this *File) Close() error {
	if this == nil || this.mapping == nil || this.closed.Swap(true) {
		return nil
	}

	mapping := this.mapping
	registryLocker.Lock()
	if mapping.refs > 0 {
		mapping.refs--
	}
	if mapping.refs > 0 {
		registryLocker.Unlock()
		return nil
	}

	if registry[mapping.path] == mapping {
		delete(registry, mapping.path)
	}
	mapping.closed = true
	data := mapping.data
	mapping.data = nil
	registryLocker.Unlock()

	return unmapData(data)
}
