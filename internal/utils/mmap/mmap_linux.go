//go:build linux

package mmap

import (
	"os"
	"syscall"
)

func mapOpenedFile(fp *os.File, size int64) ([]byte, error) {
	maxInt := int64(^uint(0) >> 1)
	if size > maxInt {
		return nil, ErrFileTooLarge
	}
	return syscall.Mmap(int(fp.Fd()), 0, int(size), syscall.PROT_READ, syscall.MAP_SHARED)
}

func unmapData(data []byte) error {
	if len(data) == 0 {
		return nil
	}
	return syscall.Munmap(data)
}

func platformFileIdentity(info os.FileInfo) (device uint64, inode uint64) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0
	}
	return uint64(stat.Dev), uint64(stat.Ino)
}
