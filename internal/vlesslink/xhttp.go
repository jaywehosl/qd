package vlesslink

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"
)

type Whole int64

func (w *Whole) UnmarshalJSON(b []byte) error {
	n, err := wholeOf(b)
	*w = Whole(n)
	return err
}

func wholeOf(b []byte) (int64, error) {
	var f float64
	if err := json.Unmarshal(b, &f); err != nil || f != math.Trunc(f) || math.Abs(f) > math.MaxInt32 {
		return 0, fmt.Errorf("%s is not a whole number", b)
	}
	return int64(f), nil
}

type Span struct{ From, To int32 }

func (s *Span) UnmarshalJSON(b []byte) error {
	if n, err := wholeOf(b); err == nil {
		s.From, s.To = int32(n), int32(n)
		return nil
	}
	var text string
	if err := json.Unmarshal(b, &text); err != nil {
		return errors.New(`a range is a number or a "from-to" string`)
	}
	if text == "" {
		*s = Span{}
		return nil
	}
	lo, hi, cut := strings.Cut(text, "-")
	from, err := strconv.ParseInt(strings.TrimSpace(lo), 10, 32)
	if err != nil {
		return fmt.Errorf("%q is not a range", text)
	}
	to := from
	if cut {
		if to, err = strconv.ParseInt(strings.TrimSpace(hi), 10, 32); err != nil {
			return fmt.Errorf("%q is not a range", text)
		}
	}
	s.From, s.To = int32(min(from, to)), int32(max(from, to))
	return nil
}

type Xmux struct {
	MaxConcurrency   Span  `json:"maxConcurrency"`
	MaxConnections   Span  `json:"maxConnections"`
	CMaxReuseTimes   Span  `json:"cMaxReuseTimes"`
	HMaxRequestTimes Span  `json:"hMaxRequestTimes"`
	HMaxReusableSecs Span  `json:"hMaxReusableSecs"`
	HKeepAlivePeriod Whole `json:"hKeepAlivePeriod"`
}

type XHTTP struct {
	Host                 string            `json:"host"`
	Path                 string            `json:"path"`
	Mode                 string            `json:"mode"`
	Headers              map[string]string `json:"headers"`
	XPaddingBytes        Span              `json:"xPaddingBytes"`
	XPaddingObfsMode     bool              `json:"xPaddingObfsMode"`
	XPaddingKey          string            `json:"xPaddingKey"`
	XPaddingHeader       string            `json:"xPaddingHeader"`
	XPaddingPlacement    string            `json:"xPaddingPlacement"`
	XPaddingMethod       string            `json:"xPaddingMethod"`
	UplinkHTTPMethod     string            `json:"uplinkHTTPMethod"`
	SessionPlacement     string            `json:"sessionPlacement"`
	SessionKey           string            `json:"sessionKey"`
	SeqPlacement         string            `json:"seqPlacement"`
	SeqKey               string            `json:"seqKey"`
	UplinkDataPlacement  string            `json:"uplinkDataPlacement"`
	UplinkDataKey        string            `json:"uplinkDataKey"`
	UplinkChunkSize      Span              `json:"uplinkChunkSize"`
	NoGRPCHeader         bool              `json:"noGRPCHeader"`
	NoSSEHeader          bool              `json:"noSSEHeader"`
	ScMaxEachPostBytes   Span              `json:"scMaxEachPostBytes"`
	ScMinPostsIntervalMs Span              `json:"scMinPostsIntervalMs"`
	ScMaxBufferedPosts   Whole             `json:"scMaxBufferedPosts"`
	ScStreamUpServerSecs Span              `json:"scStreamUpServerSecs"`
	ServerMaxHeaderBytes Whole             `json:"serverMaxHeaderBytes"`
	Xmux                 Xmux              `json:"xmux"`
	DownloadSettings     json.RawMessage   `json:"downloadSettings"`
}

func xhttpOf(q url.Values) (*XHTTP, error) {
	x := &XHTTP{}
	if extra := q.Get("extra"); extra != "" {
		if err := json.Unmarshal([]byte(extra), x); err != nil {
			return nil, fmt.Errorf("the xhttp extra does not read: %w", err)
		}
	}
	x.Host, x.Path, x.Mode = q.Get("host"), q.Get("path"), q.Get("mode")
	return x, x.settle()
}

func oneOf(field, value string, allowed ...string) error {
	for _, a := range allowed {
		if value == a {
			return nil
		}
	}
	return fmt.Errorf("the xhttp %s %q is not supported", field, value)
}

func (x *XHTTP) settle() error {
	if len(x.DownloadSettings) > 0 && string(x.DownloadSettings) != "null" {
		return errors.New("the xhttp downloadSettings are not supported yet")
	}
	if x.Mode == "" {
		x.Mode = "auto"
	}
	if err := oneOf("mode", x.Mode, "auto", "packet-up", "stream-up", "stream-one"); err != nil {
		return err
	}
	for k := range x.Headers {
		if strings.EqualFold(k, "host") {
			return errors.New("the xhttp headers cannot carry host")
		}
	}
	if x.XPaddingBytes != (Span{}) && (x.XPaddingBytes.From <= 0 || x.XPaddingBytes.To <= 0) {
		return errors.New("the xhttp xPaddingBytes cannot be disabled")
	}
	x.XPaddingKey = cmp.Or(x.XPaddingKey, "x_padding")
	x.XPaddingHeader = cmp.Or(x.XPaddingHeader, "X-Padding")
	x.XPaddingPlacement = cmp.Or(x.XPaddingPlacement, "queryInHeader")
	if err := oneOf("xPaddingPlacement", x.XPaddingPlacement, "cookie", "header", "query", "queryInHeader"); err != nil {
		return err
	}
	x.XPaddingMethod = cmp.Or(x.XPaddingMethod, "repeat-x")
	if err := oneOf("xPaddingMethod", x.XPaddingMethod, "repeat-x", "tokenish"); err != nil {
		return err
	}
	x.UplinkDataPlacement = cmp.Or(x.UplinkDataPlacement, "auto")
	if err := oneOf("uplinkDataPlacement", x.UplinkDataPlacement, "auto", "body", "cookie", "header"); err != nil {
		return err
	}
	if (x.UplinkDataPlacement == "cookie" || x.UplinkDataPlacement == "header") && x.Mode != "packet-up" {
		return errors.New("the xhttp uplink data goes in a " + x.UplinkDataPlacement + " only in packet-up mode")
	}
	x.UplinkHTTPMethod = strings.ToUpper(cmp.Or(x.UplinkHTTPMethod, "POST"))
	if x.UplinkHTTPMethod == "GET" && x.Mode != "packet-up" {
		return errors.New("the xhttp uplink goes by GET only in packet-up mode")
	}
	x.SessionPlacement = cmp.Or(x.SessionPlacement, "path")
	if err := oneOf("sessionPlacement", x.SessionPlacement, "path", "cookie", "header", "query"); err != nil {
		return err
	}
	x.SeqPlacement = cmp.Or(x.SeqPlacement, "path")
	if err := oneOf("seqPlacement", x.SeqPlacement, "path", "cookie", "header", "query"); err != nil {
		return err
	}
	x.SessionKey = cmp.Or(x.SessionKey, keyFor(x.SessionPlacement, "x_session", "X-Session"))
	x.SeqKey = cmp.Or(x.SeqKey, keyFor(x.SeqPlacement, "x_seq", "X-Seq"))
	if x.UplinkDataPlacement != "body" {
		x.UplinkDataKey = cmp.Or(x.UplinkDataKey, keyFor(x.UplinkDataPlacement, "x_data", "X-Data"))
	}
	if x.ServerMaxHeaderBytes < 0 {
		return errors.New("the xhttp serverMaxHeaderBytes cannot be negative")
	}
	if x.Xmux.MaxConnections.To > 0 && x.Xmux.MaxConcurrency.To > 0 {
		return errors.New("the xhttp xmux takes maxConnections or maxConcurrency, not both")
	}
	if x.Xmux == (Xmux{}) {
		x.Xmux.MaxConcurrency = Span{1, 1}
		x.Xmux.HMaxRequestTimes = Span{600, 900}
		x.Xmux.HMaxReusableSecs = Span{1800, 3000}
	}
	return nil
}

func keyFor(placement, plain, header string) string {
	switch placement {
	case "cookie", "query":
		return plain
	case "header", "auto":
		return header
	}
	return ""
}
