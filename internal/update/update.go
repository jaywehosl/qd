package update

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
)

const (
	Repo      = "jaywehosl/qd"
	Checksums = "checksums.txt"
	Signature = "checksums.txt.sig"
	Path      = "/qd/update/"
	publicKey = "1dq/OBvDoYanXq7Jdgh7QzF64xDoDP1lFj1Uo3mvfdw="
)

var (
	Version = "dev"
	Kind    = ""
)

func Signed(checksums, sig []byte) bool {
	key, err := base64.StdEncoding.DecodeString(publicKey)
	if err != nil || len(key) != ed25519.PublicKeySize {
		return false
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(sig)))
	if err != nil {
		return false
	}
	return ed25519.Verify(key, checksums, raw)
}

func Older(a, b string) bool {
	x, xp, xok := parse(a)
	y, yp, yok := parse(b)
	if !xok || !yok {
		return xok != yok && !xok
	}
	for i := range x {
		if x[i] != y[i] {
			return x[i] < y[i]
		}
	}
	switch {
	case xp == yp:
		return false
	case xp == "":
		return false
	case yp == "":
		return true
	}
	return xp < yp
}

func parse(tag string) ([3]int, string, bool) {
	var out [3]int
	core, pre, _ := strings.Cut(strings.TrimPrefix(tag, "v"), "-")
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return out, "", false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, "", false
		}
		out[i] = n
	}
	return out, pre, true
}

type Release struct {
	Tag       string `json:"tag"`
	Signed    bool   `json:"signed"`
	Published string `json:"published"`
}

type ghRelease struct {
	Tag       string `json:"tag_name"`
	Draft     bool   `json:"draft"`
	Published string `json:"published_at"`
	Assets    []struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

var verified sync.Map

func Releases(ctx context.Context, hc *http.Client) ([]Release, error) {
	var listed []ghRelease
	if err := getJSON(ctx, hc, "https://api.github.com/repos/"+Repo+"/releases?per_page=50", &listed); err != nil {
		return nil, err
	}

	out := []Release{}
	for _, r := range listed {
		if r.Draft || r.Tag == "" {
			continue
		}
		one := Release{Tag: r.Tag, Published: r.Published}
		var sums, sig string
		var key string
		for _, a := range r.Assets {
			switch a.Name {
			case Checksums:
				sums = a.URL
				key += fmt.Sprintf("s%d", a.ID)
			case Signature:
				sig = a.URL
				key += fmt.Sprintf("g%d", a.ID)
			}
		}
		if sums != "" && sig != "" {
			if held, ok := verified.Load(r.Tag + key); ok {
				one.Signed = held.(bool)
			} else {
				body, err1 := get(ctx, hc, sums)
				mark, err2 := get(ctx, hc, sig)
				if err1 != nil || err2 != nil {
					return nil, fmt.Errorf("%s: could not read its signature: %v %v", r.Tag, err1, err2)
				}
				one.Signed = Signed(body, mark)
				verified.Store(r.Tag+key, one.Signed)
			}
		}
		out = append(out, one)
	}
	sort.SliceStable(out, func(i, j int) bool { return Older(out[j].Tag, out[i].Tag) })
	return out, nil
}

func getJSON(ctx context.Context, hc *http.Client, url string, out any) error {
	body, err := get(ctx, hc, url)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, out)
}

func get(ctx context.Context, hc *http.Client, url string) ([]byte, error) {
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
	return io.ReadAll(io.LimitReader(rsp.Body, 4<<20))
}

const (
	HeaderVersion = "Qd-Version"
	HeaderKind    = "Qd-Kind"
)

func Stamp(h http.Header) {
	h.Set(HeaderVersion, Version)
	if Kind != "" {
		h.Set(HeaderKind, Kind)
	}
}

type Verdict string

const (
	Current  Verdict = "current"
	Behind   Verdict = "behind"
	Required Verdict = "required"
)

func Judge(own, build, target string, releases []string, dev, core bool) Verdict {
	switch {
	case target == "":
		return Current
	case build == "core" && core:
		return Current
	case !valid(own):
		if dev {
			return Current
		}
		return Required
	case !Older(own, target):
		return Current
	}
	gap := 0
	for _, tag := range releases {
		if Older(own, tag) && !Older(target, tag) {
			gap++
		}
	}
	if gap <= 1 {
		return Behind
	}
	return Required
}

func valid(tag string) bool {
	_, _, ok := parse(tag)
	return ok
}
