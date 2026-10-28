package vlesslink

import (
	"cmp"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

type Link struct {
	Name        string
	Host        string
	Port        int
	ID          string
	Flow        string
	Security    string
	SNI         string
	Fingerprint string
	PublicKey   []byte
	ShortID     []byte
	SpiderX     string
	ALPN        []string
	Insecure    bool
	Transport   string
	XHTTP       *XHTTP
	WS          *WS
}

type WS struct {
	Host string
	Path string
	Ed   uint32
}

func wsOf(q url.Values) (*WS, error) {
	ws := &WS{Host: q.Get("host"), Path: cmp.Or(q.Get("path"), "/")}
	early := q.Get("ed")
	if u, err := url.Parse(ws.Path); err == nil {
		if inner := u.Query(); inner.Has("ed") {
			early = inner.Get("ed")
			inner.Del("ed")
			u.RawQuery = inner.Encode()
			ws.Path = u.String()
		}
	}
	if early != "" {
		n, err := strconv.ParseUint(early, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("the websocket early data %q is not a number", early)
		}
		ws.Ed = uint32(n)
	}
	return ws, nil
}

const vision = "xtls-rprx-vision"

func Parse(raw string) (Link, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "vless" {
		return Link{}, errors.New("that is not a vless:// link")
	}
	if u.User == nil || u.User.Username() == "" {
		return Link{}, errors.New("the vless link carries no user id")
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil || port < 1 || port > 65535 || u.Hostname() == "" {
		return Link{}, errors.New("the vless link names no server and port")
	}
	q := u.Query()
	if enc := cmp.Or(q.Get("encryption"), "none"); enc != "none" {
		return Link{}, errors.New("vless encryption is not supported, only none")
	}

	l := Link{
		Name:        u.Fragment,
		Host:        u.Hostname(),
		Port:        port,
		ID:          u.User.Username(),
		Flow:        q.Get("flow"),
		Security:    cmp.Or(q.Get("security"), "none"),
		SNI:         q.Get("sni"),
		Fingerprint: strings.ToLower(q.Get("fp")),
		SpiderX:     cmp.Or(q.Get("spx"), "/"),
		Insecure:    q.Get("allowInsecure") == "1" || q.Get("insecure") == "1",
	}
	if alpn := q.Get("alpn"); alpn != "" {
		l.ALPN = strings.Split(alpn, ",")
	}
	switch kind := cmp.Or(q.Get("type"), "tcp"); kind {
	case "tcp", "raw":
		l.Transport = "raw"
		if header := q.Get("headerType"); header != "" && header != "none" {
			return Link{}, fmt.Errorf("the %s header is not supported", header)
		}
	case "xhttp", "splithttp":
		l.Transport = "xhttp"
		if l.XHTTP, err = xhttpOf(q); err != nil {
			return Link{}, err
		}
	case "ws", "websocket":
		l.Transport = "ws"
		if l.WS, err = wsOf(q); err != nil {
			return Link{}, err
		}
	default:
		return Link{}, fmt.Errorf("the %s transport is not supported yet, only raw, xhttp and ws", kind)
	}
	if l.Security == "reality" && l.Transport == "ws" {
		return Link{}, errors.New("reality does not run over websocket, only over raw and xhttp")
	}
	if l.Flow != "" && (l.Flow != vision || l.Transport != "raw") {
		return Link{}, fmt.Errorf("the %s flow is not supported here, only %s over raw", l.Flow, vision)
	}

	switch l.Security {
	case "reality":
		l.Fingerprint = cmp.Or(l.Fingerprint, "chrome")
		if l.SNI == "" {
			return Link{}, errors.New("the reality link names no sni")
		}
		if l.PublicKey, err = base64.RawURLEncoding.DecodeString(q.Get("pbk")); err != nil || len(l.PublicKey) != 32 {
			return Link{}, errors.New("the reality public key (pbk) is not a valid key")
		}
		sid := q.Get("sid")
		l.ShortID = make([]byte, 8)
		if len(sid) > 16 || len(sid)%2 != 0 {
			return Link{}, errors.New("the reality short id (sid) is not valid")
		}
		if _, err := hex.Decode(l.ShortID, []byte(sid)); err != nil {
			return Link{}, errors.New("the reality short id (sid) is not valid")
		}
		if !strings.HasPrefix(l.SpiderX, "/") {
			return Link{}, errors.New("the reality spider path (spx) must start with /")
		}
	case "tls":
	case "none":
		if l.Flow != "" {
			return Link{}, errors.New("the vision flow needs tls or reality")
		}
	default:
		return Link{}, fmt.Errorf("the %s security is not supported", l.Security)
	}
	return l, nil
}

func (l Link) Server() string { return net.JoinHostPort(l.Host, strconv.Itoa(l.Port)) }

func (l Link) Summary() string {
	out := l.Server() + " " + l.Transport + " " + l.Security
	if l.SNI != "" {
		out += " " + l.SNI
	}
	if l.XHTTP != nil {
		out += " " + l.XHTTP.Mode
	}
	if l.WS != nil {
		out += " " + l.WS.Path
	}
	if l.Flow != "" {
		out += " vision"
	}
	return out
}
