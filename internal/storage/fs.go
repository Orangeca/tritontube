package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
)

type FS struct{ root string }

func NewFS(root string) *FS {
	return &FS{root}

}

func (f *FS) fullPath(bucket, object string) string {
	// 防止相对路径穿越：简单拼接，真实项目可再加校验
	return filepath.Join(f.root, bucket, object)
}

func (f *FS) Put(bucket, object string, r io.Reader) (int64, string, error) {
	path := f.fullPath(bucket, object)
	if err := os.MkdirAll(filepath.Dir(path), 0o775); err != nil {
		return 0, "", err
	}
	tmp := path + ".tmp"
	fp, err := os.Create(tmp)
	if err != nil {
		return 0, "", err
	}
	defer fp.Close()
	h := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(fp, h), r)
	if copyErr != nil {
		_ = os.Remove(tmp)
		return 0, "", err
	}
	if err := fp.Sync(); err != nil {
		_ = os.Remove(tmp)
		return 0, "", err
	}
	if err := fp.Close(); err != nil {
		_ = os.Remove(tmp)
		return 0, "", err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return 0, "", err
	}
	return int64(n), hex.EncodeToString(h.Sum(nil)), nil
}

func (f *FS) Get(bucket, object string) (io.ReadCloser, error) {
	path := f.fullPath(bucket, object)
	return os.Open(path)
}
