package update

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type Opener func(name string) (io.ReadCloser, error)

var ErrCorrupt = errors.New("the downloaded package is corrupted")

func ReleaseURL(tag string) string { return "https://github.com/" + Repo + "/releases/tag/" + tag }

func Take(open Opener, name, dir string, tick func(done, total int64)) (string, error) {
	sums, err := readAll(open, Checksums, 1<<20)
	if err != nil {
		return "", err
	}
	sig, err := readAll(open, Signature, 1<<10)
	if err != nil {
		return "", err
	}
	if !Signed(sums, sig) {
		return "", fmt.Errorf("%w: the release is not signed with the update key", ErrCorrupt)
	}
	want, ok := SumOf(sums, name)
	if !ok {
		return "", fmt.Errorf("the release carries no %s", name)
	}

	body, err := open(name)
	if err != nil {
		return "", err
	}
	defer body.Close()

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, name)
	f, err := os.Create(path + ".part")
	if err != nil {
		return "", err
	}
	h := sha256.New()
	var total int64
	if sized, ok := body.(interface{ Size() int64 }); ok {
		total = sized.Size()
	}
	_, err = io.Copy(io.MultiWriter(f, h, &ticker{tick: tick, total: total}), io.LimitReader(body, 256<<20))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(path + ".part")
		return "", err
	}
	if hex.EncodeToString(h.Sum(nil)) != want {
		os.Remove(path + ".part")
		return "", fmt.Errorf("%w: %s does not match its signed checksum", ErrCorrupt, name)
	}
	os.Remove(path)
	if err := os.Rename(path+".part", path); err != nil {
		return "", err
	}
	return path, nil
}

func readAll(open Opener, name string, limit int64) ([]byte, error) {
	body, err := open(name)
	if err != nil {
		return nil, err
	}
	defer body.Close()
	return io.ReadAll(io.LimitReader(body, limit))
}

type ticker struct {
	tick  func(done, total int64)
	done  int64
	total int64
}

func (t *ticker) Write(p []byte) (int, error) {
	t.done += int64(len(p))
	if t.tick != nil {
		t.tick(t.done, t.total)
	}
	return len(p), nil
}
