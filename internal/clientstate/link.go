package clientstate

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

const linkScheme = "qd"

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

var errNotALink = errors.New("that is not a qd:// link")

func ParseLink(raw string) (Link, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Link{}, errNotALink
	}

	u, err := url.Parse(raw)
	if err != nil || u.Scheme != linkScheme {
		return Link{}, errNotALink
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

func (e Endpoint) String() string { return net.JoinHostPort(e.Address, strconv.Itoa(e.Port)) }

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
		Scheme:   linkScheme,
		User:     url.User(l.Key),
		Fragment: l.Label,
	}
	var parts []string
	if l.NetworkKey != "" {
		parts = append(parts, "k="+queryText(l.NetworkKey))
	}

	for i, e := range l.Endpoints {
		if i == 0 {
			u.Host = e.String()
		} else {
			parts = append(parts, "alt="+queryText(e.String()))
		}
	}

	u.RawQuery = strings.Join(parts, "&")
	return u.String()
}

var keepInQuery = strings.NewReplacer("%2F", "/", "%3A", ":", "%2C", ",", "%40", "@")

func queryText(s string) string { return keepInQuery.Replace(url.QueryEscape(s)) }
