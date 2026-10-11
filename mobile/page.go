package qdmobile

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/jaywehosl/qd/internal/clientapi"
	"github.com/jaywehosl/qd/internal/localapi"
	"github.com/jaywehosl/qd/internal/panel"
	"github.com/jaywehosl/qd/web"
)

var errRefused = errors.New("the system refused to establish the tunnel")

func (c *Client) Page() (string, error) {
	wire := c.wire()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.page != "" {
		return c.page, nil
	}

	files, err := web.Handler("")
	if err != nil {
		return "", err
	}

	c.seat = panel.NewSeat(c.key, c.db, wire)
	c.api.Raise, c.api.Lower = c.raise, c.lower
	routes := c.api.Routes()
	srv, err := localapi.New(localapi.Config{
		Page:    files,
		Guarded: true,
		Admin:   c.seat,
		IsAdmin: func() bool {
			sub, err := c.db.Subscription()
			return err == nil && sub.Admin
		},
		Client: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			routes.ServeHTTP(w, r)
			if r.URL.Path == "/client/api/reset" && !c.Imported() {
				forgetJournal()
			}
			if r.Method == http.MethodPost && c.host != nil {
				go c.host.Changed()
			}
		}),
		Index: func(token string) ([]byte, error) {
			return web.IndexWith(map[string]string{"X_UI_BASE_PATH": "/", "QD_TOKEN": token})
		},
	})
	if err != nil {
		return "", err
	}

	time.AfterFunc(1200*time.Millisecond, func() { c.installed() })
	l, err := srv.ListenOn("127.0.0.1", localapi.DefaultPort)
	if err != nil {
		return "", err
	}
	srv.SetFeed(c.seat.Feed)
	go http.Serve(l, srv)
	time.AfterFunc(3*time.Second, func() { c.list() })

	c.page = srv.URL() + "?t=" + srv.Token()
	say("page: served on %s", l.Addr())
	return c.page, nil
}

func (c *Client) raise() error {
	if c.Running() {
		return nil
	}

	wait := make(chan error, 1)
	c.mu.Lock()
	c.waiting = append(c.waiting, wait)
	c.mu.Unlock()

	if c.host == nil || !c.host.Raise() {
		c.settle(errRefused)
	}

	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	late := time.After(2 * time.Minute)
	for {
		select {
		case err := <-wait:
			return err
		case <-late:
			return errRefused
		case <-tick.C:
			if !c.Running() {
				continue
			}
			select {
			case err := <-wait:
				return err
			case <-time.After(time.Second):
				return nil
			}
		}
	}
}

func (c *Client) settle(err error) {
	c.mu.Lock()
	waiting := c.waiting
	c.waiting = nil
	c.mu.Unlock()

	for _, wait := range waiting {
		wait <- err
	}
}

func (c *Client) Declined() { c.settle(errRefused) }

func (c *Client) lower() error {
	if c.host == nil {
		return c.api.Disconnect()
	}
	c.host.Lower()
	for late := time.Now().Add(8 * time.Second); c.Running() && time.Now().Before(late); {
		time.Sleep(100 * time.Millisecond)
	}
	return nil
}

func (c *Client) installed() []clientapi.Process {
	c.mu.Lock()
	held, stale := c.apps, time.Since(c.appsAt) > time.Minute && !c.listing
	if held != nil && stale {
		c.listing = true
	}
	c.mu.Unlock()

	if held == nil {
		c.listed.Do(func() { c.list() })
		c.mu.Lock()
		defer c.mu.Unlock()
		return c.apps
	}
	if stale {
		go c.list()
	}
	return held
}

func (c *Client) list() []clientapi.Process {
	out := []clientapi.Process{}
	if c.host != nil {
		json.Unmarshal([]byte(c.host.Apps()), &out)
	}

	c.mu.Lock()
	c.apps, c.appsAt, c.listing = out, time.Now(), false
	c.mu.Unlock()
	return out
}
