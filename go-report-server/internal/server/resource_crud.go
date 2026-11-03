package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/acugis/acugis-report-server-go/internal/auth"
	"github.com/acugis/acugis-report-server-go/internal/contentresource"
	"github.com/acugis/acugis-report-server-go/internal/folders"
)

func (s *Server) resourcesConfigured() bool {
	return s.deps.Resources != nil && s.deps.Resources.Configured()
}

func (s *Server) handleAPIResourcesList(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if err := s.requireCatalogAccess(r, user); err != nil {
		s.writeAuthzJSON(w, err)
		return
	}
	if !s.resourcesConfigured() {
		s.writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "resources unavailable"})
		return
	}
	folderID, _ := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("folder_id")), 10, 64)
	filter := contentresource.ListFilter{
		FolderID:     folderID,
		ResourceType: strings.TrimSpace(r.URL.Query().Get("resource_type")),
		Query:        strings.TrimSpace(r.URL.Query().Get("q")),
	}
	list, err := s.deps.Resources.List(r.Context(), filter)
	if err != nil {
		s.writeResourceJSONError(w, err)
		return
	}
	out := make([]contentresource.Resource, 0, len(list))
	for _, res := range list {
		ok, err := s.canViewFolder(r, user, res.FolderID)
		if err != nil {
			s.writeAuthzJSON(w, err)
			return
		}
		if !ok {
			continue
		}
		out = append(out, res)
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (s *Server) handleAPIResourceCreate(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if err := s.requireCatalogAccess(r, user); err != nil {
		s.writeAuthzJSON(w, err)
		return
	}
	if !s.resourcesConfigured() {
		s.writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "resources unavailable"})
		return
	}

	in, err := s.parseResourceCreate(r)
	if err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	ok, err := s.canEditFolder(r, user, in.FolderID)
	if err != nil {
		s.writeAuthzJSON(w, err)
		return
	}
	if !ok {
		s.writeJSON(w, http.StatusForbidden, map[string]string{"error": "Permission denied"})
		return
	}
	if user != nil {
		in.CreatedBy = user.ID
	}
	res, err := s.deps.Resources.Create(r.Context(), in)
	if err != nil {
		s.writeResourceJSONError(w, err)
		return
	}
	s.writeJSON(w, http.StatusCreated, res)
}

func (s *Server) parseResourceCreate(r *http.Request) (contentresource.CreateInput, error) {
	var in contentresource.CreateInput
	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(strings.ToLower(ct), "application/json") {
		var body struct {
			FolderID     int64  `json:"folder_id"`
			DisplayName  string `json:"display_name"`
			ResourceType string `json:"resource_type"`
			Content      string `json:"content"`
			Filename     string `json:"original_filename"`
			Extension    string `json:"extension"`
			MimeType     string `json:"mime_type"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 8<<20)).Decode(&body); err != nil {
			return in, errors.New("invalid JSON body")
		}
		in.FolderID = body.FolderID
		in.DisplayName = body.DisplayName
		in.ResourceType = body.ResourceType
		in.OriginalFilename = body.Filename
		in.Extension = body.Extension
		in.MimeType = body.MimeType
		in.Data = []byte(body.Content)
		if in.FolderID <= 0 && s.deps.Folders != nil {
			if home, err := s.deps.Folders.EnsureHome(r.Context()); err == nil {
				in.FolderID = home.ID
			}
		}
		if len(in.Data) == 0 {
			return in, errors.New("content is required when uploading via JSON paste")
		}
		return in, nil
	}

	if err := r.ParseMultipartForm(16 << 20); err != nil {
		_ = r.ParseForm()
	}
	folderID, _ := strconv.ParseInt(firstNonEmpty(r.FormValue("folder_id"), r.URL.Query().Get("folder_id")), 10, 64)
	in.FolderID = folderID
	in.DisplayName = strings.TrimSpace(r.FormValue("display_name"))
	in.ResourceType = strings.TrimSpace(r.FormValue("resource_type"))
	in.Extension = strings.TrimSpace(r.FormValue("extension"))
	in.MimeType = strings.TrimSpace(r.FormValue("mime_type"))

	if paste := r.FormValue("content"); strings.TrimSpace(paste) != "" {
		in.Data = []byte(paste)
		in.OriginalFilename = strings.TrimSpace(r.FormValue("original_filename"))
	} else if r.MultipartForm != nil {
		filename, data, present, err := readUpload(r.MultipartForm, "file")
		if err != nil {
			return in, errors.New("failed to read uploaded file")
		}
		if present && len(data) > 0 {
			in.Data = data
			in.OriginalFilename = filename
			if in.Extension == "" {
				in.Extension = contentresource.ExtensionFromFilename(filename)
			}
			if in.MimeType == "" {
				if fhs := r.MultipartForm.File["file"]; len(fhs) > 0 {
					in.MimeType = fhs[0].Header.Get("Content-Type")
				}
			}
		}
	}
	if in.FolderID <= 0 && s.deps.Folders != nil {
		if home, err := s.deps.Folders.EnsureHome(r.Context()); err == nil {
			in.FolderID = home.ID
		}
	}
	if strings.TrimSpace(in.DisplayName) == "" {
		return in, errors.New("display_name is required")
	}
	if strings.TrimSpace(in.ResourceType) == "" {
		return in, errors.New("resource_type is required")
	}
	if len(in.Data) == 0 {
		return in, errors.New("file upload or pasted content is required")
	}
	return in, nil
}

func (s *Server) handleAPIResourceGet(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if err := s.requireCatalogAccess(r, user); err != nil {
		s.writeAuthzJSON(w, err)
		return
	}
	if !s.resourcesConfigured() {
		s.writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "resources unavailable"})
		return
	}
	id := strings.TrimSpace(chi.URLParam(r, "uuid"))
	res, err := s.deps.Resources.Get(r.Context(), id)
	if err != nil {
		s.writeResourceJSONError(w, err)
		return
	}
	ok, err := s.canViewFolder(r, user, res.FolderID)
	if err != nil {
		s.writeAuthzJSON(w, err)
		return
	}
	if !ok {
		s.writeJSON(w, http.StatusForbidden, map[string]string{"error": "Permission denied"})
		return
	}
	s.writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleAPIResourceUpdate(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if err := s.requireCatalogAccess(r, user); err != nil {
		s.writeAuthzJSON(w, err)
		return
	}
	if !s.resourcesConfigured() {
		s.writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "resources unavailable"})
		return
	}
	id := strings.TrimSpace(chi.URLParam(r, "uuid"))
	cur, err := s.deps.Resources.Get(r.Context(), id)
	if err != nil {
		s.writeResourceJSONError(w, err)
		return
	}
	ok, err := s.canEditFolder(r, user, cur.FolderID)
	if err != nil {
		s.writeAuthzJSON(w, err)
		return
	}
	if !ok {
		s.writeJSON(w, http.StatusForbidden, map[string]string{"error": "Permission denied"})
		return
	}

	var body struct {
		DisplayName  *string `json:"display_name"`
		ResourceType *string `json:"resource_type"`
		FolderID     *int64  `json:"folder_id"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if body.FolderID != nil && *body.FolderID > 0 && *body.FolderID != cur.FolderID {
		ok, err := s.canEditFolder(r, user, *body.FolderID)
		if err != nil {
			s.writeAuthzJSON(w, err)
			return
		}
		if !ok {
			s.writeJSON(w, http.StatusForbidden, map[string]string{"error": "Permission denied for destination folder"})
			return
		}
	}
	var createdBy int64
	if user != nil {
		createdBy = user.ID
	}
	res, err := s.deps.Resources.UpdateMetadata(r.Context(), id, contentresource.UpdateInput{
		DisplayName:  body.DisplayName,
		ResourceType: body.ResourceType,
		FolderID:     body.FolderID,
	}, createdBy)
	if err != nil {
		s.writeResourceJSONError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleAPIResourceDelete(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if err := s.requireCatalogAccess(r, user); err != nil {
		s.writeAuthzJSON(w, err)
		return
	}
	if !s.resourcesConfigured() {
		s.writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "resources unavailable"})
		return
	}
	id := strings.TrimSpace(chi.URLParam(r, "uuid"))
	cur, err := s.deps.Resources.Get(r.Context(), id)
	if err != nil {
		s.writeResourceJSONError(w, err)
		return
	}
	ok, err := s.canEditFolder(r, user, cur.FolderID)
	if err != nil {
		s.writeAuthzJSON(w, err)
		return
	}
	if !ok {
		s.writeJSON(w, http.StatusForbidden, map[string]string{"error": "Permission denied"})
		return
	}
	if err := s.deps.Resources.Delete(r.Context(), id); err != nil {
		s.writeResourceJSONError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "uuid": id})
}

func (s *Server) handleAPIResourceDownload(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if err := s.requireCatalogAccess(r, user); err != nil {
		s.writeAuthzJSON(w, err)
		return
	}
	if !s.resourcesConfigured() {
		s.writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "resources unavailable"})
		return
	}
	id := strings.TrimSpace(chi.URLParam(r, "uuid"))
	res, data, err := s.deps.Resources.ReadBytes(r.Context(), id)
	if err != nil {
		s.writeResourceJSONError(w, err)
		return
	}
	ok, err := s.canViewFolder(r, user, res.FolderID)
	if err != nil {
		s.writeAuthzJSON(w, err)
		return
	}
	if !ok {
		s.writeJSON(w, http.StatusForbidden, map[string]string{"error": "Permission denied"})
		return
	}
	inline := strings.EqualFold(r.URL.Query().Get("inline"), "1") ||
		strings.EqualFold(r.URL.Query().Get("disposition"), "inline")
	s.writeResourceDownload(w, res, data, inline)
}

func (s *Server) handleAPIResourceReplaceContent(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if err := s.requireCatalogAccess(r, user); err != nil {
		s.writeAuthzJSON(w, err)
		return
	}
	if !s.resourcesConfigured() {
		s.writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "resources unavailable"})
		return
	}
	id := strings.TrimSpace(chi.URLParam(r, "uuid"))
	cur, err := s.deps.Resources.Get(r.Context(), id)
	if err != nil {
		s.writeResourceJSONError(w, err)
		return
	}
	ok, err := s.canEditFolder(r, user, cur.FolderID)
	if err != nil {
		s.writeAuthzJSON(w, err)
		return
	}
	if !ok {
		s.writeJSON(w, http.StatusForbidden, map[string]string{"error": "Permission denied"})
		return
	}
	var rep contentresource.ReplaceInput
	if contentresource.IsImageType(cur.ResourceType) {
		rep, err = s.parseImageReplace(r)
	} else {
		rep, err = s.parseResourceReplace(r)
	}
	if err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	res, err := s.deps.Resources.ReplaceContent(r.Context(), id, rep)
	if err != nil {
		s.writeResourceJSONError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, res)
}

func (s *Server) parseResourceReplace(r *http.Request) (contentresource.ReplaceInput, error) {
	var in contentresource.ReplaceInput
	if err := r.ParseMultipartForm(16 << 20); err != nil {
		return in, errors.New("invalid multipart upload")
	}
	in.Extension = strings.TrimSpace(r.FormValue("extension"))
	in.MimeType = strings.TrimSpace(r.FormValue("mime_type"))
	if r.MultipartForm == nil {
		return in, errors.New("file is required")
	}
	filename, data, present, err := readUpload(r.MultipartForm, "file")
	if err != nil {
		return in, errors.New("failed to read uploaded file")
	}
	if !present || len(data) == 0 {
		return in, errors.New("file is required")
	}
	in.Data = data
	in.OriginalFilename = filename
	if in.Extension == "" {
		in.Extension = contentresource.ExtensionFromFilename(filename)
	}
	if in.MimeType == "" {
		if fhs := r.MultipartForm.File["file"]; len(fhs) > 0 {
			in.MimeType = fhs[0].Header.Get("Content-Type")
		}
	}
	return in, nil
}

func (s *Server) handleAPIResourceThumbnail(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if err := s.requireCatalogAccess(r, user); err != nil {
		s.writeAuthzJSON(w, err)
		return
	}
	if !s.resourcesConfigured() {
		s.writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "resources unavailable"})
		return
	}
	id := strings.TrimSpace(chi.URLParam(r, "uuid"))
	res, data, err := s.deps.Resources.ReadThumbnail(r.Context(), id)
	if err != nil {
		// Fall back to original for SVG (or missing thumb) so <img> preview still works.
		if errors.Is(err, contentresource.ErrNotFound) {
			res, orig, rerr := s.deps.Resources.ReadBytes(r.Context(), id)
			if rerr != nil {
				s.writeResourceJSONError(w, rerr)
				return
			}
			if !contentresource.IsImageType(res.ResourceType) {
				s.writeJSON(w, http.StatusNotFound, map[string]string{"error": "thumbnail not found"})
				return
			}
			ok, aerr := s.canViewFolder(r, user, res.FolderID)
			if aerr != nil {
				s.writeAuthzJSON(w, aerr)
				return
			}
			if !ok {
				s.writeJSON(w, http.StatusForbidden, map[string]string{"error": "Permission denied"})
				return
			}
			s.writeResourceDownload(w, res, orig, true)
			return
		}
		s.writeResourceJSONError(w, err)
		return
	}
	ok, err := s.canViewFolder(r, user, res.FolderID)
	if err != nil {
		s.writeAuthzJSON(w, err)
		return
	}
	if !ok {
		s.writeJSON(w, http.StatusForbidden, map[string]string{"error": "Permission denied"})
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "private, max-age=300")
	w.Header().Set("Content-Length", strconv.FormatInt(int64(len(data)), 10))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (s *Server) writeResourceJSONError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, contentresource.ErrNotFound):
		s.writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
	case errors.Is(err, contentresource.ErrInvalidInput), errors.Is(err, contentresource.ErrBadType),
		errors.Is(err, contentresource.ErrInvalidImage), errors.Is(err, contentresource.ErrBadImageType),
		errors.Is(err, folders.ErrBadRequest):
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	case errors.Is(err, contentresource.ErrUnavailable):
		s.writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
	default:
		s.logger.Error("content resource error", "error", err)
		s.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}
}
