package server

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/acugis/acugis-report-server-go/internal/acl"
	"github.com/acugis/acugis-report-server-go/internal/audit"
	"github.com/acugis/acugis-report-server-go/internal/auth"
	"github.com/acugis/acugis-report-server-go/internal/contentresource"
	"github.com/acugis/acugis-report-server-go/internal/explorer"
	"github.com/acugis/acugis-report-server-go/internal/folders"
)

func (s *Server) foldersConfigured() bool {
	return s.deps.Folders != nil && s.deps.Folders.Configured()
}

func (s *Server) handleAPIFoldersTree(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if err := s.requireCatalogAccess(r, user); err != nil {
		s.writeAuthzJSON(w, err)
		return
	}
	if !s.foldersConfigured() {
		s.writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "folders unavailable"})
		return
	}
	tree, err := s.deps.Folders.Tree(r.Context())
	if err != nil {
		s.logger.Error("folders tree", "error", err)
		s.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load folder tree"})
		return
	}
	filtered := s.filterTreeByACL(r, user, tree)
	s.writeJSON(w, http.StatusOK, filtered)
}

func (s *Server) filterTreeByACL(r *http.Request, user *auth.User, node *folders.TreeNode) *folders.TreeNode {
	if node == nil {
		return nil
	}
	ok, err := s.canViewFolder(r, user, node.ID)
	if err != nil || !ok {
		return nil
	}
	out := &folders.TreeNode{
		ID:       node.ID,
		ParentID: node.ParentID,
		Name:     node.Name,
		Icon:     node.Icon,
	}
	for i := range node.Children {
		child := s.filterTreeByACL(r, user, &node.Children[i])
		if child != nil {
			out.Children = append(out.Children, *child)
		}
	}
	return out
}

func (s *Server) handleAPIFoldersBrowse(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if err := s.requireCatalogAccess(r, user); err != nil {
		s.writeAuthzJSON(w, err)
		return
	}
	if !s.foldersConfigured() {
		s.writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "folders unavailable"})
		return
	}
	folderID, _ := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("folder_id")), 10, 64)
	browse, err := s.deps.Folders.Browse(r.Context(), folderID)
	if err != nil {
		s.writeFolderJSONError(w, err)
		return
	}
	ok, err := s.canViewFolder(r, user, browse.Folder.ID)
	if err != nil {
		s.writeAuthzJSON(w, err)
		return
	}
	if !ok {
		s.writeJSON(w, http.StatusForbidden, map[string]string{"error": "Permission denied"})
		return
	}

	filteredFolders := make([]folders.Folder, 0, len(browse.Folders))
	for _, f := range browse.Folders {
		ok, err := s.canViewFolder(r, user, f.ID)
		if err != nil {
			s.writeAuthzJSON(w, err)
			return
		}
		if ok {
			filteredFolders = append(filteredFolders, f)
		}
	}
	filteredItems := make([]folders.Item, 0, len(browse.Items))
	seenKeys := map[string]bool{}
	for _, it := range browse.Items {
		if it.ContentType == folders.ContentTypeReport {
			// Skip stale membership rows when the report files were already deleted.
			if s.deps.Reports != nil && !s.deps.Reports.Exists(it.ContentKey) {
				continue
			}
			if s.deps.Reports != nil && !s.allowMapReports() {
				if def, err := s.deps.Reports.Get(it.ContentKey); err == nil && def != nil && def.Type() == "map" {
					continue
				}
			}
			ok, err := s.canViewReport(r, user, it.ContentKey)
			if err != nil {
				s.writeAuthzJSON(w, err)
				return
			}
			if !ok {
				continue
			}
			it.DesignerNative, it.CanDesign = s.annotateDesignerReport(r, user, it.ContentKey)
		} else if it.ContentType == folders.ContentTypeExplorerView {
			owner, key, okRef := explorer.ParseViewRef(it.ContentKey)
			if !okRef {
				continue
			}
			view, err := s.loadExplorerView(r, owner, key)
			if err != nil {
				continue
			}
			ok, err := s.canViewExplorerView(r, user, view)
			if err != nil {
				s.writeAuthzJSON(w, err)
				return
			}
			if !ok {
				continue
			}
			if view.IsDashboard {
				if !s.allowDashboards() {
					continue
				}
				it.ResourceType = "dashboard"
			} else {
				it.ResourceType = "view"
			}
			if view.Description != "" {
				// reuse OriginalFilename field? Better add nothing — display_name is enough.
				_ = view.Description
			}
			if !view.UpdatedAt.IsZero() {
				it.UpdatedAt = view.UpdatedAt
			}
		} else {
			ok, err := s.canViewFolder(r, user, browse.Folder.ID)
			if err != nil || !ok {
				continue
			}
		}
		filteredItems = append(filteredItems, it)
		seenKeys[it.ContentType+":"+it.ContentKey] = true
	}

	if s.deps.Resources != nil {
		for i := range filteredItems {
			if filteredItems[i].ContentType != folders.ContentTypeResource {
				continue
			}
			res, err := s.deps.Resources.Get(r.Context(), filteredItems[i].ContentKey)
			if err != nil {
				continue
			}
			filteredItems[i].ResourceType = res.ResourceType
			filteredItems[i].OriginalFilename = res.OriginalFilename
			filteredItems[i].SizeBytes = res.SizeBytes
			filteredItems[i].MimeType = res.MimeType
			filteredItems[i].Extension = res.Extension
			filteredItems[i].Width = res.Width
			filteredItems[i].Height = res.Height
			if !res.UpdatedAt.IsZero() {
				filteredItems[i].UpdatedAt = res.UpdatedAt
			}
		}
	}

	// Unfiled reports appear under Home until explicitly moved.
	if browse.Folder.ParentID == nil && s.deps.Reports != nil {
		list, err := s.deps.Reports.List()
		if err == nil {
			for _, rep := range list {
				key := folders.ContentTypeReport + ":" + rep.Key
				if seenKeys[key] {
					continue
				}
				if rep.Type == "map" && !s.allowMapReports() {
					continue
				}
				if _, err := s.deps.Folders.Repo.GetItemByKey(r.Context(), folders.ContentTypeReport, rep.Key); err == nil {
					continue
				}
				ok, err := s.canViewReport(r, user, rep.Key)
				if err != nil {
					s.writeAuthzJSON(w, err)
					return
				}
				if !ok {
					continue
				}
				filteredItems = append(filteredItems, folders.Item{
					FolderID:    browse.Folder.ID,
					ContentType: folders.ContentTypeReport,
					ContentKey:  rep.Key,
					DisplayName: rep.Name,
					ACLInherit:  true,
					FolderPath:  browse.Folder.Path,
				})
			}
		}
	}

	// Unfiled Data Explorer saved views (and dashboards) are registered into
	// Home on browse so they receive a stable content_items id (not id=0).
	// Previously these were synthesized with Go zero values (id=0,
	// created_at=0001-01-01) because they existed only as filesystem SavedViews.
	if browse.Folder.ParentID == nil {
		if svc := s.explorerSvc(); svc != nil && svc.Views != nil && user != nil {
			owner := explorerOwner(user)
			views, err := svc.Views.List(r.Context(), owner)
			if err == nil {
				for i := range views {
					v := &views[i]
					ref := explorerViewRef(v)
					key := folders.ContentTypeExplorerView + ":" + ref
					if seenKeys[key] {
						continue
					}
					if _, err := s.deps.Folders.Repo.GetItemByKey(r.Context(), folders.ContentTypeExplorerView, ref); err == nil {
						continue
					}
					ok, err := s.canViewExplorerView(r, user, v)
					if err != nil {
						s.writeAuthzJSON(w, err)
						return
					}
					if !ok {
						continue
					}
					createdBy := int64(0)
					if user != nil && strings.EqualFold(strings.TrimSpace(user.Username), strings.TrimSpace(v.OwnerUsername)) {
						createdBy = user.ID
					}
					it, regErr := s.deps.Folders.RegisterExplorerViewIfMissing(r.Context(), ref, v.Name, createdBy)
					if regErr != nil || it == nil {
						s.logger.Error("register unfiled explorer view", "error", regErr, "ref", ref)
						continue
					}
					item := *it
					item.FolderPath = browse.Folder.Path
					if v.IsDashboard {
						if !s.allowDashboards() {
							continue
						}
						item.ResourceType = "dashboard"
					} else {
						item.ResourceType = "view"
					}
					if !v.UpdatedAt.IsZero() {
						item.UpdatedAt = v.UpdatedAt
					}
					filteredItems = append(filteredItems, item)
					seenKeys[key] = true
				}
			}
		}
	}

	browse.Folders = filteredFolders
	browse.Items = filteredItems
	s.writeJSON(w, http.StatusOK, browse)
}

func (s *Server) handleAPIFoldersSearch(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if err := s.requireCatalogAccess(r, user); err != nil {
		s.writeAuthzJSON(w, err)
		return
	}
	if !s.foldersConfigured() {
		s.writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "folders unavailable"})
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	hits, err := s.deps.Folders.Search(r.Context(), q, 50)
	if err != nil {
		s.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "search failed"})
		return
	}

	// Also search filesystem reports not yet in content_items.
	if s.deps.Reports != nil && q != "" {
		list, _ := s.deps.Reports.List()
		ql := strings.ToLower(q)
		for _, rep := range list {
			hay := strings.ToLower(rep.Name + " " + rep.Description + " " + rep.Key)
			if !strings.Contains(hay, ql) {
				continue
			}
			ok, err := s.canViewReport(r, user, rep.Key)
			if err != nil || !ok {
				continue
			}
			path := "Home"
			if it, err := s.deps.Folders.Repo.GetItemByKey(r.Context(), folders.ContentTypeReport, rep.Key); err == nil {
				path, _ = s.deps.Folders.Repo.PathString(r.Context(), it.FolderID)
			}
			hits = append(hits, folders.SearchHit{
				Kind:        "item",
				ContentType: folders.ContentTypeReport,
				ContentKey:  rep.Key,
				Name:        rep.Name,
				Description: rep.Description,
				FolderPath:  path,
			})
		}
	}

	// Also search the caller's unfiled Data Explorer views.
	if q != "" {
		if svc := s.explorerSvc(); svc != nil && svc.Views != nil && user != nil {
			ql := strings.ToLower(q)
			views, _ := svc.Views.List(r.Context(), explorerOwner(user))
			for i := range views {
				v := &views[i]
				hay := strings.ToLower(v.Name + " " + v.Description + " " + v.Key)
				if !strings.Contains(hay, ql) {
					continue
				}
				ok, err := s.canViewExplorerView(r, user, v)
				if err != nil || !ok {
					continue
				}
				ref := explorerViewRef(v)
				path := "Home"
				if it, err := s.deps.Folders.Repo.GetItemByKey(r.Context(), folders.ContentTypeExplorerView, ref); err == nil {
					path, _ = s.deps.Folders.Repo.PathString(r.Context(), it.FolderID)
				}
				rt := "view"
				if v.IsDashboard {
					if !s.allowDashboards() {
						continue
					}
					rt = "dashboard"
				}
				hits = append(hits, folders.SearchHit{
					Kind:         "item",
					ContentType:  folders.ContentTypeExplorerView,
					ContentKey:   ref,
					Name:         v.Name,
					Description:  v.Description,
					FolderPath:   path,
					ResourceType: rt,
				})
			}
		}
	}

	out := make([]folders.SearchHit, 0, len(hits))
	seen := map[string]bool{}
	for _, h := range hits {
		key := h.Kind + ":" + h.ContentType + ":" + h.ContentKey + ":" + strconv.FormatInt(h.ID, 10)
		if seen[key] {
			continue
		}
		seen[key] = true
		if h.Kind == "folder" {
			ok, err := s.canViewFolder(r, user, h.ID)
			if err != nil || !ok {
				continue
			}
		} else if h.ContentType == folders.ContentTypeReport {
			if s.deps.Reports != nil && !s.deps.Reports.Exists(h.ContentKey) {
				continue
			}
			ok, err := s.canViewReport(r, user, h.ContentKey)
			if err != nil || !ok {
				continue
			}
			h.DesignerNative, h.CanDesign = s.annotateDesignerReport(r, user, h.ContentKey)
		} else if h.ContentType == folders.ContentTypeExplorerView {
			owner, viewKey, okRef := explorer.ParseViewRef(h.ContentKey)
			if !okRef {
				continue
			}
			view, err := s.loadExplorerView(r, owner, viewKey)
			if err != nil {
				continue
			}
			ok, err := s.canViewExplorerView(r, user, view)
			if err != nil || !ok {
				continue
			}
			if h.ResourceType == "" {
				if view.IsDashboard {
					h.ResourceType = "dashboard"
				} else {
					h.ResourceType = "view"
				}
			}
			if view.IsDashboard && !s.allowDashboards() {
				continue
			}
		}
		out = append(out, h)
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"items": out, "count": len(out)})
}

func (s *Server) handleAPIFolderCreate(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if err := s.requireCatalogAccess(r, user); err != nil {
		s.writeAuthzJSON(w, err)
		return
	}
	if !s.foldersConfigured() {
		s.writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "folders unavailable"})
		return
	}
	var in folders.CreateFolderInput
	if err := decodeJSONBody(r, &in); err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	parentID := in.ParentID
	if parentID <= 0 {
		home, err := s.deps.Folders.Repo.GetRoot(r.Context())
		if err != nil {
			s.writeFolderJSONError(w, err)
			return
		}
		parentID = home.ID
		in.ParentID = parentID
	}
	ok, err := s.canEditFolder(r, user, parentID)
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
	f, err := s.deps.Folders.Create(r.Context(), in)
	if err != nil {
		s.recordAudit(r, audit.Event{
			Category:   audit.CategoryContent,
			Action:     audit.ActionCreate,
			ObjectType: audit.ObjectFolder,
			ObjectName: in.Name,
			Status:     audit.StatusFailure,
			Details:    map[string]any{"error": audit.SanitizeErrorMessage(err.Error())},
		})
		s.writeFolderJSONError(w, err)
		return
	}
	s.recordAudit(r, audit.Event{
		Category:   audit.CategoryContent,
		Action:     audit.ActionCreate,
		ObjectType: audit.ObjectFolder,
		ObjectID:   strconv.FormatInt(f.ID, 10),
		ObjectName: f.Name,
		Status:     audit.StatusSuccess,
	})
	s.writeJSON(w, http.StatusCreated, f)
}

func (s *Server) handleAPIFolderGet(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if err := s.requireCatalogAccess(r, user); err != nil {
		s.writeAuthzJSON(w, err)
		return
	}
	if !s.foldersConfigured() {
		s.writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "folders unavailable"})
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid folder id"})
		return
	}
	ok, err := s.canViewFolder(r, user, id)
	if err != nil {
		s.writeAuthzJSON(w, err)
		return
	}
	if !ok {
		s.writeJSON(w, http.StatusForbidden, map[string]string{"error": "Permission denied"})
		return
	}
	f, err := s.deps.Folders.Get(r.Context(), id)
	if err != nil {
		s.writeFolderJSONError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, f)
}

func (s *Server) handleAPIFolderUpdate(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if err := s.requireCatalogAccess(r, user); err != nil {
		s.writeAuthzJSON(w, err)
		return
	}
	if !s.foldersConfigured() {
		s.writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "folders unavailable"})
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid folder id"})
		return
	}
	ok, err := s.canEditFolder(r, user, id)
	if err != nil {
		s.writeAuthzJSON(w, err)
		return
	}
	if !ok {
		s.writeJSON(w, http.StatusForbidden, map[string]string{"error": "Permission denied"})
		return
	}
	var in folders.UpdateFolderInput
	if err := decodeJSONBody(r, &in); err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	f, err := s.deps.Folders.Update(r.Context(), id, in)
	if err != nil {
		s.recordAudit(r, audit.Event{
			Category:   audit.CategoryContent,
			Action:     audit.ActionUpdate,
			ObjectType: audit.ObjectFolder,
			ObjectID:   strconv.FormatInt(id, 10),
			Status:     audit.StatusFailure,
			Details:    map[string]any{"error": audit.SanitizeErrorMessage(err.Error())},
		})
		s.writeFolderJSONError(w, err)
		return
	}
	s.recordAudit(r, audit.Event{
		Category:   audit.CategoryContent,
		Action:     audit.ActionUpdate,
		ObjectType: audit.ObjectFolder,
		ObjectID:   strconv.FormatInt(f.ID, 10),
		ObjectName: f.Name,
		Status:     audit.StatusSuccess,
	})
	s.writeJSON(w, http.StatusOK, f)
}

func (s *Server) handleAPIFolderDelete(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if err := s.requireCatalogAccess(r, user); err != nil {
		s.writeAuthzJSON(w, err)
		return
	}
	if !s.foldersConfigured() {
		s.writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "folders unavailable"})
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid folder id"})
		return
	}
	ok, err := s.canAdminFolder(r, user, id)
	if err != nil {
		s.writeAuthzJSON(w, err)
		return
	}
	if !ok {
		s.writeJSON(w, http.StatusForbidden, map[string]string{"error": "Permission denied"})
		return
	}
	q := r.URL.Query()
	opts := folders.DeleteOptions{
		Recursive:      q.Get("recursive") == "1" || strings.EqualFold(q.Get("recursive"), "true"),
		Confirm:        q.Get("confirm") == "1" || strings.EqualFold(q.Get("confirm"), "true"),
		AllowRecursive: s.cfg.FolderAllowRecursiveDelete,
	}
	if err := s.deps.Folders.DeleteFolder(r.Context(), id, opts); err != nil {
		s.recordAudit(r, audit.Event{
			Category:   audit.CategoryContent,
			Action:     audit.ActionDelete,
			ObjectType: audit.ObjectFolder,
			ObjectID:   strconv.FormatInt(id, 10),
			Status:     audit.StatusFailure,
			Details:    map[string]any{"error": audit.SanitizeErrorMessage(err.Error())},
		})
		s.writeFolderJSONError(w, err)
		return
	}
	s.recordAudit(r, audit.Event{
		Category:   audit.CategoryContent,
		Action:     audit.ActionDelete,
		ObjectType: audit.ObjectFolder,
		ObjectID:   strconv.FormatInt(id, 10),
		Status:     audit.StatusSuccess,
	})
	s.writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleAPIFolderMove(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if err := s.requireCatalogAccess(r, user); err != nil {
		s.writeAuthzJSON(w, err)
		return
	}
	if !s.foldersConfigured() {
		s.writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "folders unavailable"})
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid folder id"})
		return
	}
	ok, err := s.canEditFolder(r, user, id)
	if err != nil {
		s.writeAuthzJSON(w, err)
		return
	}
	if !ok {
		s.writeJSON(w, http.StatusForbidden, map[string]string{"error": "Permission denied"})
		return
	}
	var in folders.MoveFolderInput
	if err := decodeJSONBody(r, &in); err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	ok, err = s.canEditFolder(r, user, in.ParentID)
	if err != nil {
		s.writeAuthzJSON(w, err)
		return
	}
	if !ok {
		s.writeJSON(w, http.StatusForbidden, map[string]string{"error": "Permission denied"})
		return
	}
	f, err := s.deps.Folders.MoveFolder(r.Context(), id, in)
	if err != nil {
		s.recordAudit(r, audit.Event{
			Category:   audit.CategoryContent,
			Action:     audit.ActionMove,
			ObjectType: audit.ObjectFolder,
			ObjectID:   strconv.FormatInt(id, 10),
			Status:     audit.StatusFailure,
			Details: map[string]any{
				"parent_id": in.ParentID,
				"error":     audit.SanitizeErrorMessage(err.Error()),
			},
		})
		s.writeFolderJSONError(w, err)
		return
	}
	s.recordAudit(r, audit.Event{
		Category:   audit.CategoryContent,
		Action:     audit.ActionMove,
		ObjectType: audit.ObjectFolder,
		ObjectID:   strconv.FormatInt(f.ID, 10),
		ObjectName: f.Name,
		Status:     audit.StatusSuccess,
		Details:    map[string]any{"parent_id": in.ParentID},
	})
	s.writeJSON(w, http.StatusOK, f)
}

func (s *Server) handleAPIFolderDuplicate(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if err := s.requireCatalogAccess(r, user); err != nil {
		s.writeAuthzJSON(w, err)
		return
	}
	if !s.foldersConfigured() {
		s.writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "folders unavailable"})
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid folder id"})
		return
	}
	src, err := s.deps.Folders.Get(r.Context(), id)
	if err != nil {
		s.writeFolderJSONError(w, err)
		return
	}
	if src.ParentID == nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cannot duplicate Home"})
		return
	}
	ok, err := s.canEditFolder(r, user, *src.ParentID)
	if err != nil {
		s.writeAuthzJSON(w, err)
		return
	}
	if !ok {
		s.writeJSON(w, http.StatusForbidden, map[string]string{"error": "Permission denied"})
		return
	}
	var createdBy int64
	if user != nil {
		createdBy = user.ID
	}
	f, err := s.deps.Folders.DuplicateFolder(r.Context(), id, createdBy)
	if err != nil {
		s.writeFolderJSONError(w, err)
		return
	}
	s.writeJSON(w, http.StatusCreated, f)
}

func (s *Server) handleAPIFolderPermissions(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if err := s.requireCatalogAccess(r, user); err != nil {
		s.writeAuthzJSON(w, err)
		return
	}
	if !s.foldersConfigured() {
		s.writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "folders unavailable"})
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid folder id"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		ok, err := s.canViewFolder(r, user, id)
		if err != nil {
			s.writeAuthzJSON(w, err)
			return
		}
		if !ok {
			s.writeJSON(w, http.StatusForbidden, map[string]string{"error": "Permission denied"})
			return
		}
		groups, err := s.deps.Folders.Repo.ListFolderPermissionGroups(r.Context(), id)
		if err != nil {
			s.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list permissions"})
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]any{"group_names": groups})
	case http.MethodPut:
		ok, err := s.canAdminFolder(r, user, id)
		if err != nil {
			s.writeAuthzJSON(w, err)
			return
		}
		if !ok {
			s.writeJSON(w, http.StatusForbidden, map[string]string{"error": "Permission denied"})
			return
		}
		var in folders.SetPermissionsInput
		if err := decodeJSONBody(r, &in); err != nil {
			s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}
		if err := s.deps.Folders.Repo.ReplaceFolderPermissions(r.Context(), id, in.GroupNames, in.Permission); err != nil {
			s.recordAudit(r, audit.Event{
				Category:   audit.CategorySecurity,
				Action:     audit.ActionChange,
				ObjectType: audit.ObjectPermission,
				ObjectID:   strconv.FormatInt(id, 10),
				Status:     audit.StatusFailure,
				Details: map[string]any{
					"target_type": audit.ObjectFolder,
					"target_id":   id,
					"error":       audit.SanitizeErrorMessage(err.Error()),
				},
			})
			s.writeFolderJSONError(w, err)
			return
		}
		perm := strings.TrimSpace(in.Permission)
		if perm == "" {
			perm = "view"
		}
		s.recordAudit(r, audit.Event{
			Category:   audit.CategorySecurity,
			Action:     audit.ActionChange,
			ObjectType: audit.ObjectPermission,
			ObjectID:   strconv.FormatInt(id, 10),
			Status:     audit.StatusSuccess,
			Details: map[string]any{
				"target_type":    audit.ObjectFolder,
				"target_id":      id,
				"principal_type": "group",
				"group_names":    in.GroupNames,
				"permission":     perm,
			},
		})
		s.writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		s.writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (s *Server) handleAPIContentMove(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if err := s.requireCatalogAccess(r, user); err != nil {
		s.writeAuthzJSON(w, err)
		return
	}
	if !s.foldersConfigured() {
		s.writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "folders unavailable"})
		return
	}
	var in folders.MoveItemInput
	if err := decodeJSONBody(r, &in); err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
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
	if in.ContentType == folders.ContentTypeReport {
		canEdit, err := s.canEditReport(r, user, in.ContentKey)
		if err != nil {
			s.writeAuthzJSON(w, err)
			return
		}
		if !canEdit && !s.isPlatformAdmin(user) {
			s.writeJSON(w, http.StatusForbidden, map[string]string{"error": "Permission denied"})
			return
		}
		if in.DisplayName == "" && s.deps.Reports != nil {
			if def, err := s.deps.Reports.Get(in.ContentKey); err == nil {
				in.DisplayName = stringify(def.Raw["name"])
				if in.DisplayName == "" {
					in.DisplayName = in.ContentKey
				}
			}
		}
	}
	if in.ContentType == folders.ContentTypeDashboard {
		// Legacy alias — store as explorer_view.
		in.ContentType = folders.ContentTypeExplorerView
	}
	if in.ContentType == folders.ContentTypeExplorerView {
		owner, key, okRef := explorer.ParseViewRef(in.ContentKey)
		if !okRef {
			s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "content_key must be owner/viewKey"})
			return
		}
		view, err := s.loadExplorerView(r, owner, key)
		if err != nil {
			s.writeExplorerErr(w, err)
			return
		}
		canEdit, err := s.canEditExplorerView(r, user, view)
		if err != nil {
			s.writeAuthzJSON(w, err)
			return
		}
		if !canEdit && !s.isPlatformAdmin(user) {
			s.writeJSON(w, http.StatusForbidden, map[string]string{"error": "Permission denied"})
			return
		}
		in.ContentKey = explorerViewRef(view)
		if in.DisplayName == "" {
			in.DisplayName = view.Name
			if in.DisplayName == "" {
				in.DisplayName = in.ContentKey
			}
		}
	}
	if user != nil {
		in.CreatedBy = user.ID
	}
	oldFolderID := int64(0)
	if in.ContentType == folders.ContentTypeReport && s.deps.Folders != nil && s.deps.Folders.Repo != nil {
		if prev, err := s.deps.Folders.Repo.GetItemByKey(r.Context(), folders.ContentTypeReport, in.ContentKey); err == nil && prev != nil {
			oldFolderID = prev.FolderID
		}
	}
	it, err := s.deps.Folders.MoveContent(r.Context(), in)
	if err != nil {
		s.writeFolderJSONError(w, err)
		return
	}
	if in.ContentType == folders.ContentTypeResource && s.deps.Resources != nil {
		folderID := in.FolderID
		_, _ = s.deps.Resources.UpdateMetadata(r.Context(), in.ContentKey, contentresource.UpdateInput{
			FolderID:    &folderID,
			DisplayName: strPtrIfNonEmpty(in.DisplayName),
		}, in.CreatedBy)
	}
	if in.ContentType == folders.ContentTypeReport {
		newFolderID := in.FolderID
		if it != nil && it.FolderID > 0 {
			newFolderID = it.FolderID
		}
		s.applyResourceBaseOnMove(r.Context(), in.ContentKey, oldFolderID, newFolderID)
	}
	s.writeJSON(w, http.StatusOK, it)
}

func strPtrIfNonEmpty(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}

// syncReportResourceBase writes the shared catalog-folder resource base.
// Designer per-report bases must not go through this helper: it would replace
// "DavidG/testingone" with "DavidG". Publish and folder move call it only for
// legacy, missing, or malformed metadata.
func (s *Server) syncReportResourceBase(ctx context.Context, reportKey string, folderID int64) {
	if s.deps.Reports == nil {
		return
	}
	if s.deps.Folders == nil && s.testSlashFolder == nil {
		return
	}
	slash := s.slashForFolder(ctx, folderID)
	if err := s.deps.Reports.SaveResourceBase(reportKey, slash); err != nil {
		s.logger.Warn("save report resource base", "report", reportKey, "folder", slash, "error", err)
	} else {
		s.logger.Info("saved report resource base", "report", reportKey, "folder", strings.Trim(slash, "/"))
	}
}

func (s *Server) handleAPIFavorites(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if err := s.requireCatalogAccess(r, user); err != nil {
		s.writeAuthzJSON(w, err)
		return
	}
	if !s.foldersConfigured() || user == nil || user.ID <= 0 {
		s.writeJSON(w, http.StatusOK, map[string]any{"items": []any{}})
		return
	}
	switch r.Method {
	case http.MethodGet:
		items, err := s.deps.Folders.Repo.ListFavorites(r.Context(), user.ID)
		if err != nil {
			s.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list favorites"})
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]any{"items": items})
	case http.MethodPost:
		var body struct {
			ContentType string `json:"content_type"`
			ContentKey  string `json:"content_key"`
			Favorite    *bool  `json:"favorite"`
		}
		if err := decodeJSONBody(r, &body); err != nil {
			s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}
		fav := true
		if body.Favorite != nil {
			fav = *body.Favorite
		}
		if err := s.deps.Folders.Repo.SetFavorite(r.Context(), user.ID, body.ContentType, body.ContentKey, fav); err != nil {
			s.writeFolderJSONError(w, err)
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "favorite": fav})
	default:
		s.writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (s *Server) canViewFolder(r *http.Request, user *auth.User, folderID int64) (bool, error) {
	if s.cfg.AuthDisabled || (s.deps.Auth != nil && s.deps.Auth.AuthDisabled()) {
		return true, nil
	}
	if s.isPlatformAdmin(user) {
		return true, nil
	}
	if s.deps.ACL == nil {
		return false, errStoreUnavailable
	}
	d, err := s.deps.ACL.CheckPermission(r.Context(), user, acl.ResourceTypeContentFolder, folders.FolderResourceKey(folderID), acl.PermissionView)
	if err != nil {
		return false, errStoreUnavailable
	}
	return d.Allowed, nil
}

func (s *Server) canEditFolder(r *http.Request, user *auth.User, folderID int64) (bool, error) {
	if s.cfg.AuthDisabled || (s.deps.Auth != nil && s.deps.Auth.AuthDisabled()) {
		return true, nil
	}
	if s.isPlatformAdmin(user) {
		return true, nil
	}
	if s.deps.ACL == nil {
		return false, errStoreUnavailable
	}
	d, err := s.deps.ACL.CheckPermission(r.Context(), user, acl.ResourceTypeContentFolder, folders.FolderResourceKey(folderID), acl.PermissionEdit)
	if err != nil {
		return false, errStoreUnavailable
	}
	return d.Allowed, nil
}

func (s *Server) canAdminFolder(r *http.Request, user *auth.User, folderID int64) (bool, error) {
	if s.cfg.AuthDisabled || (s.deps.Auth != nil && s.deps.Auth.AuthDisabled()) {
		return true, nil
	}
	if s.isPlatformAdmin(user) {
		return true, nil
	}
	if s.deps.ACL == nil {
		return false, errStoreUnavailable
	}
	d, err := s.deps.ACL.CheckPermission(r.Context(), user, acl.ResourceTypeContentFolder, folders.FolderResourceKey(folderID), acl.PermissionAdmin)
	if err != nil {
		return false, errStoreUnavailable
	}
	return d.Allowed, nil
}

func (s *Server) writeFolderJSONError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, folders.ErrNotFound), errors.Is(err, folders.ErrItemNotFound):
		s.writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
	case errors.Is(err, folders.ErrBadRequest), errors.Is(err, folders.ErrRootImmutable):
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	case errors.Is(err, folders.ErrConflict), errors.Is(err, folders.ErrCycle):
		s.writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
	case errors.Is(err, folders.ErrNotEmpty):
		s.writeJSON(w, http.StatusConflict, map[string]string{"error": "folder is not empty; delete children first or use recursive=1&confirm=1"})
	case errors.Is(err, folders.ErrConfirmRequired):
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "recursive delete requires confirm=1"})
	case errors.Is(err, folders.ErrRecursiveDenied):
		s.writeJSON(w, http.StatusForbidden, map[string]string{"error": "recursive delete is disabled (set JRS_FOLDER_ALLOW_RECURSIVE_DELETE=1)"})
	case errors.Is(err, folders.ErrUnavailable):
		s.writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
	default:
		s.logger.Error("folders error", "error", err)
		s.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}
}
