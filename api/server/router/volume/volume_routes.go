package volume

import (
	"encoding/json"
	"net/http"

	"github.com/docker/docker/api/server/httputils"
	"github.com/docker/engine-api/types"
	"golang.org/x/net/context"
)

// swagger:route GET /volumes volumes getVolumesList
//
// List volumes
//
// Produces:
// - application/json
// Responses:
//   200: body:VolumesListResponse
//   500: body:ErrorResponse
func (v *volumeRouter) getVolumesList(ctx context.Context, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	if err := httputils.ParseForm(r); err != nil {
		return err
	}

	volumes, warnings, err := v.backend.Volumes(r.Form.Get("filters"))
	if err != nil {
		return err
	}
	return httputils.WriteJSON(w, http.StatusOK, &types.VolumesListResponse{Volumes: volumes, Warnings: warnings})
}

// swagger:route GET /volumes/{name} volumes getVolumeByName
//
// Get detailed information about a volume
//
// Produces:
// - application/json
// Responses:
//   200: body:Volume
//   404: body:ErrorResponse
//   500: body:ErrorResponse
func (v *volumeRouter) getVolumeByName(ctx context.Context, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	if err := httputils.ParseForm(r); err != nil {
		return err
	}

	volume, err := v.backend.VolumeInspect(vars["name"])
	if err != nil {
		return err
	}
	return httputils.WriteJSON(w, http.StatusOK, volume)
}

// swagger:route POST /volumes/create volumes postVolumesCreate
//
// Create a volume
//
// Consumes:
// - application/json
// Produces:
// - application/json
// Responses:
//   201: noError
//   500: body:ErrorResponse
func (v *volumeRouter) postVolumesCreate(ctx context.Context, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	if err := httputils.ParseForm(r); err != nil {
		return err
	}

	if err := httputils.CheckForJSON(r); err != nil {
		return err
	}

	var req types.VolumeCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return err
	}

	volume, err := v.backend.VolumeCreate(req.Name, req.Driver, req.DriverOpts, req.Labels)
	if err != nil {
		return err
	}
	return httputils.WriteJSON(w, http.StatusCreated, volume)
}

// swagger:route DELETE /volumes/{name} volumes deleteVolumes
//
// Remove a volume
//
// Produces:
// - application/json
// Responses:
//   204: noError
//   404: body:ErrorResponse
//   409: body:ErrorResponse
//   500: body:ErrorResponse
func (v *volumeRouter) deleteVolumes(ctx context.Context, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	if err := httputils.ParseForm(r); err != nil {
		return err
	}
	if err := v.backend.VolumeRm(vars["name"]); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
