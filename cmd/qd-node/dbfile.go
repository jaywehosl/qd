//go:build linux

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/jaywehosl/quic-diver/internal/store"
)


func (state *controlState) dbRead(req request) response {
	var body struct {
		Offset int64 `json:"offset"`
	}
	json.Unmarshal(req.Body, &body)

	snapshot := state.dbPath + ".snapshot"
	if body.Offset == 0 {
		os.Remove(snapshot)
		if _, err := state.db.SQL().Exec(`VACUUM INTO ?`, snapshot); err != nil {
			return response{OK: false, Error: err.Error()}
		}
		state.gaveAt = time.Now()
		state.gaveChunks = 0
	}
	state.gaveChunks++

	file, err := os.Open(snapshot)
	if err != nil {
		return response{OK: false, Error: err.Error()}
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return response{OK: false, Error: err.Error()}
	}

	buf := make([]byte, dbChunk)
	n, err := file.ReadAt(buf, body.Offset)
	if err != nil && n == 0 && body.Offset < info.Size() {
		return response{OK: false, Error: err.Error()}
	}

	done := body.Offset+int64(n) >= info.Size()
	answer := map[string]any{
		"size":  info.Size(),
		"chunk": buf[:n],
		"eof":   done,
	}
	if done {
		answer["sum"] = sumOf(snapshot)
		defer os.Remove(snapshot)
		fmt.Printf("database   copy out: %d B in %d chunks, %s\n",
			info.Size(), state.gaveChunks, time.Since(state.gaveAt).Round(time.Millisecond))
	}

	return reply(req, answer)
}

func sumOf(path string) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()

	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return ""
	}
	return hex.EncodeToString(digest.Sum(nil))
}

func (state *controlState) dbWrite(req request) response {
	var body struct {
		Offset int64  `json:"offset"`
		Chunk  []byte `json:"chunk"`
		EOF    bool   `json:"eof"`
		Sum    string `json:"sum"`
	}
	if err := json.Unmarshal(req.Body, &body); err != nil {
		return response{OK: false, Error: err.Error()}
	}

	incoming := state.dbPath + ".incoming"
	flags := os.O_CREATE | os.O_WRONLY
	if body.Offset == 0 {
		flags |= os.O_TRUNC
	}
	file, err := os.OpenFile(incoming, flags, 0o600)
	if err != nil {
		return response{OK: false, Error: err.Error()}
	}
	if _, err := file.WriteAt(body.Chunk, body.Offset); err != nil {
		file.Close()
		return response{OK: false, Error: err.Error()}
	}
	file.Close()

	if body.Offset == 0 {
		state.tookAt = time.Now()
		state.tookChunks = 0
	}
	state.tookChunks++

	if !body.EOF {
		return reply(req, map[string]any{"offset": body.Offset + int64(len(body.Chunk))})
	}
	fmt.Printf("database   copy in: %d B in %d chunks, %s\n",
		body.Offset+int64(len(body.Chunk)), state.tookChunks,
		time.Since(state.tookAt).Round(time.Millisecond))

	if body.Sum != "" {
		if got := sumOf(incoming); got != body.Sum {
			os.Remove(incoming)
			return response{OK: false,
				Error: fmt.Sprintf("the copy did not survive the trip: expected %s, got %s", body.Sum[:12], got[:12])}
		}
	}

	if err := checkDatabase(incoming); err != nil {
		os.Remove(incoming)
		return response{OK: false, Error: err.Error()}
	}

	state.db.Close()
	for _, suffix := range []string{"-wal", "-shm"} {
		os.Remove(state.dbPath + suffix)
	}
	if err := os.Rename(incoming, state.dbPath); err != nil {
		return response{OK: false, Error: err.Error()}
	}

	db, err := store.Open(state.dbPath)
	if err != nil {
		return response{OK: false, Error: err.Error()}
	}
	state.db = db

	state.applySelf()
	state.syncSessions()
	state.reloadResolver()

	revision, _ := db.Version()
	return reply(req, map[string]any{"restored": true, "revision": revision})
}

func checkDatabase(path string) error {
	probe, err := store.Open(path)
	if err != nil {
		return err
	}
	defer probe.Close()
	if _, err := probe.Nodes(); err != nil {
		return err
	}
	_, err = probe.Version()
	return err
}
