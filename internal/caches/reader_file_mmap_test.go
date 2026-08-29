package caches

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	mmaputils "github.com/TeaOSLab/EdgeNode/internal/utils/mmap"
)

func TestMMAPFileReader(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("当前 mmap 缓存读取只在 Linux 启用")
	}

	urlData := []byte("https://example.com/demo")
	headerData := []byte("Content-Type: text/plain\r\nX-Test: mmap\r\n")
	bodyData := []byte("0123456789abcdefghijklmnopqrstuvwxyz")

	meta := make([]byte, SizeMeta)
	binary.BigEndian.PutUint32(meta[OffsetExpiresAt:OffsetExpiresAt+SizeExpiresAt], 2_000_000_000)
	copy(meta[OffsetStatus:OffsetStatus+SizeStatus], []byte("200"))
	binary.BigEndian.PutUint32(meta[OffsetURLLength:OffsetURLLength+SizeURLLength], uint32(len(urlData)))
	binary.BigEndian.PutUint32(meta[OffsetHeaderLength:OffsetHeaderLength+SizeHeaderLength], uint32(len(headerData)))
	binary.BigEndian.PutUint64(meta[OffsetBodyLength:OffsetBodyLength+SizeBodyLength], uint64(len(bodyData)))

	cacheData := make([]byte, 0, len(meta)+len(urlData)+len(headerData)+len(bodyData))
	cacheData = append(cacheData, meta...)
	cacheData = append(cacheData, urlData...)
	cacheData = append(cacheData, headerData...)
	cacheData = append(cacheData, bodyData...)

	path := filepath.Join(t.TempDir(), "demo.cache")
	if err := os.WriteFile(path, cacheData, 0o644); err != nil {
		t.Fatal(err)
	}

	mapped, err := mmaputils.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	reader := NewMMAPFileReader(mapped)
	if err = reader.Init(); err != nil {
		t.Fatal(err)
	}
	defer reader.Close()

	if reader.TypeName() != "mmap" {
		t.Fatalf("unexpected reader type: %s", reader.TypeName())
	}
	if reader.Status() != 200 {
		t.Fatalf("unexpected status: %d", reader.Status())
	}
	if reader.HeaderSize() != int64(len(headerData)) {
		t.Fatalf("unexpected header size: %d", reader.HeaderSize())
	}
	if reader.BodySize() != int64(len(bodyData)) {
		t.Fatalf("unexpected body size: %d", reader.BodySize())
	}

	buf := make([]byte, 7)
	var header bytes.Buffer
	if err = reader.ReadHeader(buf, func(n int) (bool, error) {
		_, _ = header.Write(buf[:n])
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(header.Bytes(), headerData) {
		t.Fatalf("unexpected header: %q", header.Bytes())
	}

	var body bytes.Buffer
	if err = reader.ReadBody(buf, func(n int) (bool, error) {
		_, _ = body.Write(buf[:n])
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(body.Bytes(), bodyData) {
		t.Fatalf("unexpected body: %q", body.Bytes())
	}

	var bodyRange bytes.Buffer
	if err = reader.ReadBodyRange(buf, 10, 19, func(n int) (bool, error) {
		_, _ = bodyRange.Write(buf[:n])
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bodyRange.Bytes(), bodyData[10:20]) {
		t.Fatalf("unexpected body range: %q", bodyRange.Bytes())
	}

	var copied bytes.Buffer
	n, err := reader.CopyBodyTo(&copied)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(bodyData) || !bytes.Equal(copied.Bytes(), bodyData) {
		t.Fatalf("unexpected copied body: n=%d body=%q", n, copied.Bytes())
	}
}
