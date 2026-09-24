package clientapi

import (
	"bytes"
	"compress/flate"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/jaywehosl/quic-diver/internal/clientstate"
)

const rulesTag = "qdr1."

type rulesFile struct {
	OS      string      `json:"os"`
	Default string      `json:"d"`
	Rules   [][3]string `json:"r"`
}

var errNotRules = errors.New("this is not a qd routing file, or it is damaged")

func (a *API) ExportRules() (string, error) {
	rules, err := a.db.Rules()
	if err != nil {
		return "", err
	}
	def, err := a.db.DefaultRole()
	if err != nil {
		return "", err
	}

	f := rulesFile{OS: runtime.GOOS, Default: def, Rules: [][3]string{}}
	for _, r := range rules {
		f.Rules = append(f.Rules, [3]string{r.Process, r.Path, r.Role})
	}
	raw, err := json.Marshal(f)
	if err != nil {
		return "", err
	}

	var packed bytes.Buffer
	w, err := flate.NewWriter(&packed, flate.BestCompression)
	if err != nil {
		return "", err
	}
	w.Write(raw)
	w.Close()
	return rulesTag + base64.RawURLEncoding.EncodeToString(packed.Bytes()), nil
}

func (a *API) ImportRules(code string) (int, error) {
	body, ok := strings.CutPrefix(strings.TrimSpace(code), rulesTag)
	if !ok {
		return 0, errNotRules
	}
	packed, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil {
		return 0, errNotRules
	}
	raw, err := io.ReadAll(io.LimitReader(flate.NewReader(bytes.NewReader(packed)), 1<<20))
	if err != nil {
		return 0, errNotRules
	}
	var f rulesFile
	if err := json.Unmarshal(raw, &f); err != nil {
		return 0, errNotRules
	}
	if f.OS != runtime.GOOS {
		return 0, fmt.Errorf("these rules were saved on %s and cannot be applied on %s", osName(f.OS), osName(runtime.GOOS))
	}

	rules := make([]clientstate.Rule, 0, len(f.Rules))
	for _, r := range f.Rules {
		rules = append(rules, clientstate.Rule{Process: r[0], Path: r[1], Role: r[2]})
	}
	if err := a.db.ReplaceRules(f.Default, rules); err != nil {
		return 0, err
	}
	a.platform.RulesChanged()
	return len(rules), nil
}

func osName(goos string) string {
	switch goos {
	case "windows":
		return "Windows"
	case "linux":
		return "Linux"
	case "android":
		return "Android"
	case "":
		return "an unknown system"
	}
	return goos
}

func (a *API) exportRules(w http.ResponseWriter, r *http.Request) {
	code, err := a.ExportRules()
	if err != nil {
		fail(w, err)
		return
	}
	name := "qd-routing-" + runtime.GOOS + "-" + time.Now().Format("2006-01-02_15-04-05") + ".qdr"
	ok(w, map[string]any{"code": code, "name": name})
}

func (a *API) importRules(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<20)).Decode(&body); err != nil {
		fail(w, errNotRules)
		return
	}
	n, err := a.ImportRules(body.Code)
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, map[string]any{"rules": n})
}
