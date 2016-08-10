package image

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/docker/docker/api/server/httputils"
	"github.com/docker/docker/api/types/backend"
	"github.com/docker/docker/pkg/ioutils"
	"github.com/docker/docker/pkg/streamformatter"
	"github.com/docker/docker/registry"
	"github.com/docker/engine-api/types"
	"github.com/docker/engine-api/types/container"
	"github.com/docker/engine-api/types/versions"
	"golang.org/x/net/context"
)

// swagger:route POST /commit images postCommit
//
// Create a new image from a container’s changes
//
//   Consumes:
//   - application/json
//   Produces:
//   - application/json
//   Responses:
//     201: body:ContainerCommitResponse
//     404: body:ErrorResponse
//     500: body:ErrorResponse
func (s *imageRouter) postCommit(ctx context.Context, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	if err := httputils.ParseForm(r); err != nil {
		return err
	}

	if err := httputils.CheckForJSON(r); err != nil {
		return err
	}

	cname := r.Form.Get("container")

	pause := httputils.BoolValue(r, "pause")
	version := httputils.VersionFromContext(ctx)
	if r.FormValue("pause") == "" && versions.GreaterThanOrEqualTo(version, "1.13") {
		pause = true
	}

	c, _, _, err := s.decoder.DecodeConfig(r.Body)
	if err != nil && err != io.EOF { //Do not fail if body is empty.
		return err
	}
	if c == nil {
		c = &container.Config{}
	}

	commitCfg := &backend.ContainerCommitConfig{
		ContainerCommitConfig: types.ContainerCommitConfig{
			Pause:        pause,
			Repo:         r.Form.Get("repo"),
			Tag:          r.Form.Get("tag"),
			Author:       r.Form.Get("author"),
			Comment:      r.Form.Get("comment"),
			Config:       c,
			MergeConfigs: true,
		},
		Changes: r.Form["changes"],
	}

	imgID, err := s.backend.Commit(cname, commitCfg)
	if err != nil {
		return err
	}

	return httputils.WriteJSON(w, http.StatusCreated, &types.ContainerCommitResponse{
		ID: string(imgID),
	})
}

// swagger:route POST /images/create images postImagesCreate
//
// Create an image either by pulling it from the registry or by importing it
//
//   Consumes:
//   - application/json
//   Produces:
//   - application/json
//   Responses:
//     200: noError
//     500: body:ErrorResponse
func (s *imageRouter) postImagesCreate(ctx context.Context, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	if err := httputils.ParseForm(r); err != nil {
		return err
	}

	var (
		image   = r.Form.Get("fromImage")
		repo    = r.Form.Get("repo")
		tag     = r.Form.Get("tag")
		message = r.Form.Get("message")
		err     error
		output  = ioutils.NewWriteFlusher(w)
	)
	defer output.Close()

	w.Header().Set("Content-Type", "application/json")

	if image != "" { //pull
		metaHeaders := map[string][]string{}
		for k, v := range r.Header {
			if strings.HasPrefix(k, "X-Meta-") {
				metaHeaders[k] = v
			}
		}

		authEncoded := r.Header.Get("X-Registry-Auth")
		authConfig := &types.AuthConfig{}
		if authEncoded != "" {
			authJSON := base64.NewDecoder(base64.URLEncoding, strings.NewReader(authEncoded))
			if err := json.NewDecoder(authJSON).Decode(authConfig); err != nil {
				// for a pull it is not an error if no auth was given
				// to increase compatibility with the existing api it is defaulting to be empty
				authConfig = &types.AuthConfig{}
			}
		}

		err = s.backend.PullImage(ctx, image, tag, metaHeaders, authConfig, output)
	} else { //import
		src := r.Form.Get("fromSrc")
		// 'err' MUST NOT be defined within this block, we need any error
		// generated from the download to be available to the output
		// stream processing below
		err = s.backend.ImportImage(src, repo, tag, message, r.Body, output, r.Form["changes"])
	}
	if err != nil {
		if !output.Flushed() {
			return err
		}
		sf := streamformatter.NewJSONStreamFormatter()
		output.Write(sf.FormatError(err))
	}

	return nil
}

// swagger:route POST /images/{name}/push images postImagesCreate
//
// Push an image to the Docker Hub or a private registry
//
// If you wish to push an image on to a private registry, that image must
// already have a tag into a repository which references that registry hostname
// and port. This repository name should then be used in the URL. This
// duplicates the command line’s flow.
//
// The push is cancelled if the HTTP connection is closed.
//
// TODO: refactor ImagePullOptions to use here
//
//   Consumes:
//   - application/json
//   Produces:
//   - application/json
//   Responses:
//     200: noError
//     404: body:ErrorResponse
//     500: body:ErrorResponse
func (s *imageRouter) postImagesPush(ctx context.Context, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	metaHeaders := map[string][]string{}
	for k, v := range r.Header {
		if strings.HasPrefix(k, "X-Meta-") {
			metaHeaders[k] = v
		}
	}
	if err := httputils.ParseForm(r); err != nil {
		return err
	}
	authConfig := &types.AuthConfig{}

	authEncoded := r.Header.Get("X-Registry-Auth")
	if authEncoded != "" {
		// the new format is to handle the authConfig as a header
		authJSON := base64.NewDecoder(base64.URLEncoding, strings.NewReader(authEncoded))
		if err := json.NewDecoder(authJSON).Decode(authConfig); err != nil {
			// to increase compatibility to existing api it is defaulting to be empty
			authConfig = &types.AuthConfig{}
		}
	} else {
		// the old format is supported for compatibility if there was no authConfig header
		if err := json.NewDecoder(r.Body).Decode(authConfig); err != nil {
			return fmt.Errorf("Bad parameters and missing X-Registry-Auth: %v", err)
		}
	}

	image := vars["name"]
	tag := r.Form.Get("tag")

	output := ioutils.NewWriteFlusher(w)
	defer output.Close()

	w.Header().Set("Content-Type", "application/json")

	if err := s.backend.PushImage(ctx, image, tag, metaHeaders, authConfig, output); err != nil {
		if !output.Flushed() {
			return err
		}
		sf := streamformatter.NewJSONStreamFormatter()
		output.Write(sf.FormatError(err))
	}
	return nil
}

// swagger:route GET /images/{name}/get images getImagesGet
//
// Get a tarball containing all images and metadata for a repository
//
// If name is a specific name and tag (e.g. ubuntu:latest), then only that
// image (and its parents) are returned. If name is an image ID, similarly only
// that image (and its parents) are returned, but with the exclusion of the
// ‘repositories’ file in the tarball, as there were no image names referenced.
//
//   Produces:
//   - application/x-tar
//   - application/json
//   Responses:
//     200: noError
//     500: body:ErrorResponse
func (s *imageRouter) getImagesGet(ctx context.Context, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	if err := httputils.ParseForm(r); err != nil {
		return err
	}

	w.Header().Set("Content-Type", "application/x-tar")

	output := ioutils.NewWriteFlusher(w)
	defer output.Close()
	var names []string
	if name, ok := vars["name"]; ok {
		names = []string{name}
	} else {
		names = r.Form["names"]
	}

	if err := s.backend.ExportImage(names, output); err != nil {
		if !output.Flushed() {
			return err
		}
		sf := streamformatter.NewJSONStreamFormatter()
		output.Write(sf.FormatError(err))
	}
	return nil
}

// swagger:route POST /images/load images postImagesLoad
//
// Load a set of images and tags into a Docker repository
//
//	 Consumes:
//   - application/x-tar
//   Produces:
//   - application/json
//   Responses:
//     200: noError
//     500: body:ErrorResponse
func (s *imageRouter) postImagesLoad(ctx context.Context, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	if err := httputils.ParseForm(r); err != nil {
		return err
	}
	quiet := httputils.BoolValueOrDefault(r, "quiet", true)

	if !quiet {
		w.Header().Set("Content-Type", "application/json")

		output := ioutils.NewWriteFlusher(w)
		defer output.Close()
		if err := s.backend.LoadImage(r.Body, output, quiet); err != nil {
			output.Write(streamformatter.NewJSONStreamFormatter().FormatError(err))
		}
		return nil
	}
	return s.backend.LoadImage(r.Body, w, quiet)
}

// swagger:response
type deleteImagesResponse struct {
	// in:body
	Body []*types.ImageDelete
}

// swagger:route DELETE /images/{name} images deleteImages
//
// Remove an image
//
//   Produces:
//   - application/json
//   Responses:
//     200: deleteImagesResponse
//     404: body:ErrorResponse
//     409: body:ErrorResponse
//     500: body:ErrorResponse
func (s *imageRouter) deleteImages(ctx context.Context, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	if err := httputils.ParseForm(r); err != nil {
		return err
	}

	name := vars["name"]

	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("image name cannot be blank")
	}

	force := httputils.BoolValue(r, "force")
	prune := !httputils.BoolValue(r, "noprune")

	list, err := s.backend.ImageDelete(name, force, prune)
	if err != nil {
		return err
	}

	return httputils.WriteJSON(w, http.StatusOK, list)
}

// swagger:route GET /images/{name}/json images getImagesByName
//
// Get detailed information about an image
//
//   Produces:
//   - application/json
//   Responses:
//     200: body:ImageInspect
//     404: body:ErrorResponse
//     500: body:ErrorResponse
func (s *imageRouter) getImagesByName(ctx context.Context, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	imageInspect, err := s.backend.LookupImage(vars["name"])
	if err != nil {
		return err
	}

	return httputils.WriteJSON(w, http.StatusOK, imageInspect)
}

// swagger:route GET /images/json images getImages
//
// List images
//
//   Produces:
//   - application/json
//   Responses:
//     200: body:ImageInspect
//     500: body:ErrorResponse
func (s *imageRouter) getImages(ctx context.Context, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	if err := httputils.ParseForm(r); err != nil {
		return err
	}

	// FIXME: The filter parameter could just be a match filter
	images, err := s.backend.Images(r.Form.Get("filters"), r.Form.Get("filter"), httputils.BoolValue(r, "all"))
	if err != nil {
		return err
	}

	return httputils.WriteJSON(w, http.StatusOK, images)
}

// swagger:response
type getImagesHistoryResponse struct {
	// in: body
	Body []*types.ImageHistory
}

// swagger:route GET /images/{name}/history images getImagesHistory
//
// Get the history of an image
//
//   Produces:
//   - application/json
//   Responses:
//     200: getImagesHistoryResponse
//     404: body:ErrorResponse
//     500: body:ErrorResponse
func (s *imageRouter) getImagesHistory(ctx context.Context, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	name := vars["name"]
	history, err := s.backend.ImageHistory(name)
	if err != nil {
		return err
	}

	return httputils.WriteJSON(w, http.StatusOK, history)
}

// swagger:route POST /images/{name}/tag images postImagesTag
//
// Tag an image with a repository
//
//   Produces:
//   - application/json
//   Responses:
//     201: noError
//     400: body:ErrorResponse
//     404: body:ErrorResponse
//     409: body:ErrorResponse
//     500: body:ErrorResponse
func (s *imageRouter) postImagesTag(ctx context.Context, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	if err := httputils.ParseForm(r); err != nil {
		return err
	}
	if err := s.backend.TagImage(vars["name"], r.Form.Get("repo"), r.Form.Get("tag")); err != nil {
		return err
	}
	w.WriteHeader(http.StatusCreated)
	return nil
}

// swagger:route GET /images/search images getImagesSearch
//
// Search for an image on Docker Hub
//
// TODO: is response actually correct?
//
//   Produces:
//   - application/json
//   Responses:
//     200: body:SearchResults
//     500: body:ErrorResponse
func (s *imageRouter) getImagesSearch(ctx context.Context, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	if err := httputils.ParseForm(r); err != nil {
		return err
	}
	var (
		config      *types.AuthConfig
		authEncoded = r.Header.Get("X-Registry-Auth")
		headers     = map[string][]string{}
	)

	if authEncoded != "" {
		authJSON := base64.NewDecoder(base64.URLEncoding, strings.NewReader(authEncoded))
		if err := json.NewDecoder(authJSON).Decode(&config); err != nil {
			// for a search it is not an error if no auth was given
			// to increase compatibility with the existing api it is defaulting to be empty
			config = &types.AuthConfig{}
		}
	}
	for k, v := range r.Header {
		if strings.HasPrefix(k, "X-Meta-") {
			headers[k] = v
		}
	}
	limit := registry.DefaultSearchLimit
	if r.Form.Get("limit") != "" {
		limitValue, err := strconv.Atoi(r.Form.Get("limit"))
		if err != nil {
			return err
		}
		limit = limitValue
	}
	query, err := s.backend.SearchRegistryForImages(ctx, r.Form.Get("filters"), r.Form.Get("term"), limit, config, headers)
	if err != nil {
		return err
	}
	return httputils.WriteJSON(w, http.StatusOK, query.Results)
}
