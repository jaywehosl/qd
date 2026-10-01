package update

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var Assets = map[string]bool{
	"qd-client-windows-amd64.exe":  true,
	"qd-client-linux-amd64.tar.gz": true,
	"qd-android-arm64.apk":         true,
	Checksums:                      true,
	Signature:                      true,
}

type Shelf struct {
	Dir    string
	Client *http.Client
	Target func() string
	Log    func(format string, args ...any)

	mu      sync.Mutex
	pending map[string]*sync.Mutex
}

func (s *Shelf) Serve(w http.ResponseWriter, r *http.Request, tag, name string) {
	if s == nil || s.Target == nil || tag == "" || tag != s.Target() || !Assets[name] {
		http.NotFound(w, r)
		return
	}
	path, err := s.hold(r.Context(), tag, name)
	if err != nil {
		if s.Log != nil {
			s.Log("update     %s/%s is not on the shelf: %v", tag, name, err)
		}
		http.Error(w, "the node could not fetch this update", http.StatusBadGateway)
		return
	}
	f, err := os.Open(path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	http.ServeContent(w, r, name, info.ModTime(), f)
}

func (s *Shelf) hold(ctx context.Context, tag, name string) (string, error) {
	lock := s.lockFor(tag + "/" + name)
	lock.Lock()
	defer lock.Unlock()

	dir := filepath.Join(s.Dir, filepath.Base(tag))
	path := filepath.Join(dir, name)
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}

	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Minute)
	defer cancel()
	base := "https://github.com/" + Repo + "/releases/download/" + tag + "/"
	sums, err := s.fetch(ctx, base+Checksums, 1<<20)
	if err != nil {
		return "", err
	}
	sig, err := s.fetch(ctx, base+Signature, 1<<10)
	if err != nil {
		return "", err
	}
	if !Signed(sums, sig) {
		return "", errors.New("checksums.txt is not signed with the update key")
	}

	body := sums
	switch name {
	case Signature:
		body = sig
	case Checksums:
	default:
		want, ok := SumOf(sums, name)
		if !ok {
			return "", fmt.Errorf("checksums.txt names no %s", name)
		}
		if body, err = s.fetch(ctx, base+name, 256<<20); err != nil {
			return "", err
		}
		if got := sha256.Sum256(body); hex.EncodeToString(got[:]) != want {
			return "", fmt.Errorf("%s does not match its checksum", name)
		}
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	tmp := path + ".part"
	if err := os.WriteFile(tmp, body, 0o644); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, path); err != nil {
		return "", err
	}
	if s.Log != nil {
		s.Log("update     %s/%s is on the shelf, %d bytes", tag, name, len(body))
	}
	return path, nil
}

func (s *Shelf) lockFor(key string) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pending == nil {
		s.pending = map[string]*sync.Mutex{}
	}
	if s.pending[key] == nil {
		s.pending[key] = &sync.Mutex{}
	}
	return s.pending[key]
}

func (s *Shelf) Stock(tag string) {
	if s == nil || tag == "" {
		return
	}
	ctx := context.Background()
	path, err := s.hold(ctx, tag, Checksums)
	if err != nil {
		s.say("update     %s could not be stocked: %v", tag, err)
		return
	}
	sums, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for name := range Assets {
		if _, listed := SumOf(sums, name); !listed && name != Signature {
			continue
		}
		if _, err := s.hold(ctx, tag, name); err != nil {
			s.say("update     %s/%s could not be stocked: %v", tag, name, err)
		}
	}
	old, _ := os.ReadDir(s.Dir)
	for _, e := range old {
		if e.IsDir() && e.Name() != filepath.Base(tag) {
			os.RemoveAll(filepath.Join(s.Dir, e.Name()))
		}
	}
}

func (s *Shelf) say(format string, args ...any) {
	if s.Log != nil {
		s.Log(format, args...)
	}
}

func (s *Shelf) fetch(ctx context.Context, url string, limit int64) ([]byte, error) {
	var err error
	for try := 0; try < 3; try++ {
		var body []byte
		if body, err = s.fetchOnce(ctx, url, limit); err == nil {
			return body, nil
		}
		select {
		case <-ctx.Done():
			return nil, err
		case <-time.After(3 * time.Second):
		}
	}
	return nil, err
}

func (s *Shelf) fetchOnce(ctx context.Context, url string, limit int64) ([]byte, error) {
	hc := s.Client
	if hc == nil {
		hc = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	rsp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", url, rsp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(rsp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("%s is larger than %d bytes", url, limit)
	}
	return body, nil
}

func SumOf(checksums []byte, name string) (string, bool) {
	lines := bufio.NewScanner(bytes.NewReader(checksums))
	for lines.Scan() {
		sum, file, ok := strings.Cut(strings.TrimSpace(lines.Text()), " ")
		if ok && strings.TrimPrefix(strings.TrimSpace(file), "*") == name {
			return sum, true
		}
	}
	return "", false
}
