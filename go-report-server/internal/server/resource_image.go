package server

import (
	"errors"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/acugis/acugis-report-server-go/internal/auth"
	"github.com/acugis/acugis-report-server-go/internal/contentresource"
)

// Image Resource REST API — used by Report Server UI and other AcuGIS products.
// Images are content resources with resource_type=image; folder ACL is inherited.

func (s *Server) handleAPIImageResourcesList(w http.ResponseWriter, r *http.Request) {
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
		ResourceType: contentresource.TypeImage,
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
	s.writeJSON(w, http.StatusOK, map[string]any{
		"items":            out,
		"supported_formats": contentresource.ImageFormatNames(),
	})
}

func (s *Server) handleAPIImageResourceCreate(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if err := s.requireCatalogAccess(r, user); err != nil {
		s.writeAuthzJSON(w, err)
		return
	}
	if !s.resourcesConfigured() {
		s.writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "resources unavailable"})
		return
	}
	in, err := s.parseImageUpload(r)
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
	in.ResourceType = contentresource.TypeImage
	res, err := s.deps.Resources.Create(r.Context(), in)
	if err != nil {
		s.writeResourceJSONError(w, err)
		return
	}
	s.writeJSON(w, http.StatusCreated, res)
}

func (s *Server) handleAPIImageResourceGet(w http.ResponseWriter, r *http.Request) {
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
	if !contentresource.IsImageType(res.ResourceType) {
		s.writeJSON(w, http.StatusNotFound, map[string]string{"error": "image resource not found"})
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

func (s *Server) handleAPIImageResourceReplace(w http.ResponseWriter, r *http.Request) {
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
	if !contentresource.IsImageType(cur.ResourceType) {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "resource is not an image"})
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
	rep, err := s.parseImageReplace(r)
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

func (s *Server) handleAPIImageResourceDelete(w http.ResponseWriter, r *http.Request) {
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
	if !contentresource.IsImageType(cur.ResourceType) {
		s.writeJSON(w, http.StatusNotFound, map[string]string{"error": "image resource not found"})
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

func (s *Server) handleAPIImageResourceDownload(w http.ResponseWriter, r *http.Request) {
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
	if !contentresource.IsImageType(res.ResourceType) {
		s.writeJSON(w, http.StatusNotFound, map[string]string{"error": "image resource not found"})
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

func (s *Server) parseImageUpload(r *http.Request) (contentresource.CreateInput, error) {
	var in contentresource.CreateInput
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		return in, errors.New("invalid multipart upload (max 32 MiB)")
	}
	folderID, _ := strconv.ParseInt(firstNonEmpty(r.FormValue("folder_id"), r.URL.Query().Get("folder_id")), 10, 64)
	in.FolderID = folderID
	in.DisplayName = strings.TrimSpace(r.FormValue("display_name"))
	in.Extension = strings.TrimSpace(r.FormValue("extension"))
	in.MimeType = strings.TrimSpace(r.FormValue("mime_type"))
	in.ResourceType = contentresource.TypeImage

	if r.MultipartForm == nil {
		return in, errors.New("image file is required")
	}
	filename, data, present, err := readUpload(r.MultipartForm, "file")
	if err != nil {
		return in, errors.New("failed to read uploaded file")
	}
	if !present || len(data) == 0 {
		return in, errors.New("image file is required")
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
	if in.FolderID <= 0 && s.deps.Folders != nil {
		if home, err := s.deps.Folders.EnsureHome(r.Context()); err == nil {
			in.FolderID = home.ID
		}
	}
	if strings.TrimSpace(in.DisplayName) == "" {
		base := filepath.Base(filename)
		if ext := filepath.Ext(base); ext != "" {
			base = strings.TrimSuffix(base, ext)
		}
		in.DisplayName = strings.TrimSpace(base)
	}
	if strings.TrimSpace(in.DisplayName) == "" {
		return in, errors.New("display_name is required")
	}
	return in, nil
}

func (s *Server) parseImageReplace(r *http.Request) (contentresource.ReplaceInput, error) {
	var in contentresource.ReplaceInput
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		return in, errors.New("invalid multipart upload (max 32 MiB)")
	}
	in.Extension = strings.TrimSpace(r.FormValue("extension"))
	in.MimeType = strings.TrimSpace(r.FormValue("mime_type"))
	if r.MultipartForm == nil {
		return in, errors.New("image file is required")
	}
	filename, data, present, err := readUpload(r.MultipartForm, "file")
	if err != nil {
		return in, errors.New("failed to read uploaded file")
	}
	if !present || len(data) == 0 {
		return in, errors.New("image file is required")
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

func (s *Server) writeResourceDownload(w http.ResponseWriter, res *contentresource.Resource, data []byte, inline bool) {
	name := res.OriginalFilename
	if name == "" {
		name = res.DisplayName + res.Extension
	}
	name = filepath.Base(name)
	mime := res.MimeType
	if mime == "" {
		mime = "application/octet-stream"
	}
	disp := "attachment"
	if inline {
		disp = "inline"
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Content-Disposition", disp+`; filename="`+strings.ReplaceAll(name, `"`, ``)+`"`)
	w.Header().Set("Content-Length", strconv.FormatInt(int64(len(data)), 10))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
