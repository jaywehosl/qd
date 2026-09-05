package clientstate

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

const LinkScheme = "qd"

type Endpoint struct {
	Address string
	Port    int
}

type Link struct {
	Key        string
	Label      string
	NetworkKey string
	Endpoints  []Endpoint
}

var ErrNotALink = errors.New("that is not a qd:// link")

func ParseLink(raw string) (Link, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Link{}, ErrNotALink
	}

	u, err := url.Parse(raw)
	if err != nil || u.Scheme != LinkScheme {
		return Link{}, ErrNotALink
	}
	if u.User == nil || u.User.Username() == "" {
		return Link{}, errors.New("the link carries no key")
	}

	primary, err := parseEndpoint(u.Host)
	if err != nil {
		return Link{}, err
	}

	link := Link{
		Key:        u.User.Username(),
		Label:      u.Fragment,
		NetworkKey: u.Query().Get("k"),
		Endpoints:  []Endpoint{primary},
	}

	for _, alt := range u.Query()["alt"] {
		e, err := parseEndpoint(alt)
		if err != nil {
			continue
		}
		link.Endpoints = append(link.Endpoints, e)
	}
	return link, nil
}

func parseEndpoint(hostPort string) (Endpoint, error) {
	if hostPort == "" {
		return Endpoint{}, errors.New("the link names no node")
	}

	host, port, err := net.SplitHostPort(hostPort)
	if err != nil {
		return Endpoint{Address: hostPort, Port: 443}, nil
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return Endpoint{}, fmt.Errorf("%q is not a port", port)
	}
	return Endpoint{Address: host, Port: n}, nil
}

func (l Link) String() string {
	u := url.URL{
		Scheme:   LinkScheme,
		User:     url.User(l.Key),
		Fragment: l.Label,
	}
	if len(l.Endpoints) > 0 {
		u.Host = net.JoinHostPort(l.Endpoints[0].Address, strconv.Itoa(l.Endpoints[0].Port))
	}
	q := url.Values{}
	for _, e := range l.Endpoints[1:] {
		q.Add("alt", net.JoinHostPort(e.Address, strconv.Itoa(e.Port)))
	}
	if l.NetworkKey != "" {
		q.Set("k", l.NetworkKey)
	}
	if len(q) > 0 {
		u.RawQuery = q.Encode()
	}
	return u.String()
}
