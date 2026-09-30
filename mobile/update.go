package qdmobile

import (
	"encoding/json"
	"errors"

	"github.com/jaywehosl/qd/internal/clientapi"
)

func (c *Client) UpdateJSON() string {
	raw, _ := json.Marshal(c.api.UpdateInfo())
	return string(raw)
}

func (c *Client) CheckUpdate() string {
	err := c.api.CheckAndUpdate()
	switch {
	case err == nil:
		return ""
	case errors.Is(err, clientapi.ErrUpToDate):
		return "latest"
	}
	return err.Error()
}

func (c *Client) PostponeUpdate(minutes int) string {
	if err := c.api.Postpone(minutes); err != nil {
		return err.Error()
	}
	return ""
}

func (c *Client) FakeUpdate(state string) { c.api.Fake(state) }
