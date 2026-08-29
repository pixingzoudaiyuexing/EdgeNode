//go:build !linux

package mmap

import "os"

func mapOpenedFile(fp *os.File, size int64) ([]byte, error) {
	return nil, ErrUnsupported
}

func unmapData(data []byte) error {
	return nil
}

func platformFileIdentity(info os.FileInfo) (device uint64, inode uint64) {
	return 0, 0
}
