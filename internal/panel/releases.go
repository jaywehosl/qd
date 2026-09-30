package panel

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jaywehosl/qd/internal/update"
)

var releaseClient = &http.Client{Timeout: 20 * time.Second}

func (a *API) releases(w http.ResponseWriter, r *http.Request) {
	list, err := update.Releases(r.Context(), releaseClient)
	if err != nil {
		sendFail(w, err)
		return
	}
	sendOK(w, list)
}

func releasesFor(ctx context.Context, want string) (string, error) {
	if want == "" {
		return "", nil
	}
	list, err := update.Releases(ctx, releaseClient)
	if err != nil {
		return "", err
	}
	tags := make([]string, 0, len(list))
	found := false
	for _, r := range list {
		tags = append(tags, r.Tag)
		if r.Tag != want {
			continue
		}
		if !r.Signed {
			return "", fmt.Errorf("%s is not signed with the update key, clients would refuse it", want)
		}
		found = true
	}
	if !found {
		return "", fmt.Errorf("no release %s on GitHub", want)
	}
	return strings.Join(tags, "\n"), nil
}
