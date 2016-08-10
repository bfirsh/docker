package system

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/docker/api"
	"github.com/docker/docker/api/server/httputils"
	"github.com/docker/docker/errors"
	"github.com/docker/docker/pkg/ioutils"
	"github.com/docker/engine-api/types"
	"github.com/docker/engine-api/types/events"
	"github.com/docker/engine-api/types/filters"
	timetypes "github.com/docker/engine-api/types/time"
	"golang.org/x/net/context"
)

func optionsHandler(ctx context.Context, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	w.WriteHeader(http.StatusOK)
	return nil
}

// swagger:route GET /_ping misc pingHandler
//
// Ping the server
//
// This is a dummy endpoint you can use to test if the server is accessible.
//
//   Produces:
//   - text/plain
//   Responses:
//     200: noError
//     500: body:ErrorResponse
func pingHandler(ctx context.Context, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	_, err := w.Write([]byte{'O', 'K'})
	return err
}

// swagger:route GET /info misc getInfo
//
// Get system-wide information
//
//   Produces:
//   - application/json
//   Responses:
//     200: body:Info
//     500: body:ErrorResponse
func (s *systemRouter) getInfo(ctx context.Context, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	info, err := s.backend.SystemInfo()
	if err != nil {
		return err
	}
	if s.clusterProvider != nil {
		info.Swarm = s.clusterProvider.Info()
	}

	return httputils.WriteJSON(w, http.StatusOK, info)
}

// swagger:route GET /version misc getVersion
//
// Get Docker version
//
// This returns the version of Docker that is running and various information
// about the system that Docker is running on.
//
//   Produces:
//   - application/json
//   Responses:
//     200: body:Version
//     500: body:ErrorResponse
func (s *systemRouter) getVersion(ctx context.Context, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	info := s.backend.SystemVersion()
	info.APIVersion = api.DefaultVersion

	return httputils.WriteJSON(w, http.StatusOK, info)
}

// swagger:route GET /events misc getEvents
//
// Monitor events
//
// Events are a stream of all the things that a Docker Engine is doing. They
// can be received in real time with streaming, or by polling using the `since` // parameter.
//
//   Produces:
//   - application/json
//   Responses:
//     200: noError
//     500: body:ErrorResponse
func (s *systemRouter) getEvents(ctx context.Context, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	if err := httputils.ParseForm(r); err != nil {
		return err
	}

	since, err := eventTime(r.Form.Get("since"))
	if err != nil {
		return err
	}
	until, err := eventTime(r.Form.Get("until"))
	if err != nil {
		return err
	}

	var (
		timeout        <-chan time.Time
		onlyPastEvents bool
	)
	if !until.IsZero() {
		if until.Before(since) {
			return errors.NewBadRequestError(fmt.Errorf("`since` time (%s) cannot be after `until` time (%s)", r.Form.Get("since"), r.Form.Get("until")))
		}

		now := time.Now()

		onlyPastEvents = until.Before(now)

		if !onlyPastEvents {
			dur := until.Sub(now)
			timeout = time.NewTimer(dur).C
		}
	}

	ef, err := filters.FromParam(r.Form.Get("filters"))
	if err != nil {
		return err
	}

	w.Header().Set("Content-Type", "application/json")
	output := ioutils.NewWriteFlusher(w)
	defer output.Close()
	output.Flush()

	enc := json.NewEncoder(output)

	buffered, l := s.backend.SubscribeToEvents(since, until, ef)
	defer s.backend.UnsubscribeFromEvents(l)

	for _, ev := range buffered {
		if err := enc.Encode(ev); err != nil {
			return err
		}
	}

	if onlyPastEvents {
		return nil
	}

	for {
		select {
		case ev := <-l:
			jev, ok := ev.(events.Message)
			if !ok {
				logrus.Warnf("unexpected event message: %q", ev)
				continue
			}
			if err := enc.Encode(jev); err != nil {
				return err
			}
		case <-timeout:
			return nil
		case <-ctx.Done():
			logrus.Debug("Client context cancelled, stop sending events")
			return nil
		}
	}
}

// swagger:route POST /auth misc postAuth
//
// Validate credentials for a registry

// This validates the credentials for a registry and gets an identity token,
// if available, for accessing the registry without password.
//
//   Consumes:
//   - application/json
//   Produces:
//   - application/json
//   Responses:
//     200: body:AuthResponse
//     500: body:ErrorResponse
func (s *systemRouter) postAuth(ctx context.Context, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	var config *types.AuthConfig
	err := json.NewDecoder(r.Body).Decode(&config)
	r.Body.Close()
	if err != nil {
		return err
	}
	status, token, err := s.backend.AuthenticateToRegistry(ctx, config)
	if err != nil {
		return err
	}
	return httputils.WriteJSON(w, http.StatusOK, &types.AuthResponse{
		Status:        status,
		IdentityToken: token,
	})
}

func eventTime(formTime string) (time.Time, error) {
	t, tNano, err := timetypes.ParseTimestamps(formTime, -1)
	if err != nil {
		return time.Time{}, err
	}
	if t == -1 {
		return time.Time{}, nil
	}
	return time.Unix(t, tNano), nil
}
