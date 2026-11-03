package server

import (
	_ "embed"
	"errors"
	"html/template"
	"net/http"
	"strconv"
	"strings"

	"github.com/acugis/acugis-report-server-go/internal/admin"
	"github.com/acugis/acugis-report-server-go/internal/audit"
	"github.com/acugis/acugis-report-server-go/internal/auth"
)

//go:embed admin_groups.html
var adminGroupsHTML string

type adminGroupsPageData struct {
	materialChromeData
	UsersURL    string
	GroupsURL   string
	Message     string
	Error       string
	EditID      int64
	EditGroup   *admin.GroupRow
	EditMembers map[int64]bool
	AllUsers    []admin.UserRef
	Groups      []admin.GroupRow
}

var adminGroupsTemplate = template.Must(template.New("admin_groups").Funcs(template.FuncMap{
	"containsInt": func(m map[int64]bool, id int64) bool {
		return m != nil && m[id]
	},
	"protectedGroup": admin.IsProtectedGroupName,
}).Parse(adminGroupsHTML))

func (s *Server) handleAdminGroupsPage(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if err := s.requirePlatformAdmin(r, user); err != nil {
		s.writeAuthzHTML(w, r, err)
		return
	}

	data := adminGroupsPageData{
		materialChromeData: s.materialChrome("Groups"),
		UsersURL:           s.goURL("/admin/users"),
		GroupsURL:          s.goURL("/admin/groups"),
	}

	repo := s.adminRepo()
	if repo == nil || !repo.Configured() {
		data.Error = "Unable to connect to the application PostgreSQL database. Check config.php or JRS_DB_* environment variables."
		s.renderAdminGroups(w, data)
		return
	}

	editID, _ := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("id")), 10, 64)
	data.EditID = editID

	if r.Method == http.MethodPost {
		s.handleAdminGroupsPOST(w, r, repo, &data)
		return
	}

	s.loadAdminGroupsPage(r, repo, &data)
	s.renderAdminGroups(w, data)
}

func (s *Server) handleAdminGroupsPOST(w http.ResponseWriter, r *http.Request, repo *admin.Repository, data *adminGroupsPageData) {
	if err := r.ParseForm(); err != nil {
		data.Error = "Failed to parse form."
		s.loadAdminGroupsPage(r, repo, data)
		s.renderAdminGroups(w, *data)
		return
	}

	action := strings.TrimSpace(r.FormValue("action"))
	id, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("id")), 10, 64)

	if action == "delete" && id > 0 {
		if err := repo.DeleteGroup(r.Context(), id); err != nil {
			if errors.Is(err, admin.ErrProtectedGroup) {
				data.Error = "Built-in groups Administrators and Public cannot be deleted."
			} else {
				data.Error = "Failed to delete group."
			}
			s.recordAudit(r, audit.Event{
				Category:   audit.CategorySecurity,
				Action:     audit.ActionDelete,
				ObjectType: audit.ObjectGroup,
				ObjectID:   strconv.FormatInt(id, 10),
				Status:     audit.StatusFailure,
				Details:    map[string]any{"error": audit.SanitizeErrorMessage(err.Error())},
			})
		} else {
			data.Message = "Group deleted."
			data.EditID = 0
			s.recordAudit(r, audit.Event{
				Category:   audit.CategorySecurity,
				Action:     audit.ActionDelete,
				ObjectType: audit.ObjectGroup,
				ObjectID:   strconv.FormatInt(id, 10),
				Status:     audit.StatusSuccess,
			})
		}
		s.loadAdminGroupsPage(r, repo, data)
		s.renderAdminGroups(w, *data)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	description := strings.TrimSpace(r.FormValue("description"))
	memberIDs := parseInt64Slice(r.Form["members"])

	if name == "" {
		data.Error = "Group name is required."
		data.EditID = id
	} else {
		saveAction := audit.ActionCreate
		if id > 0 {
			saveAction = audit.ActionUpdate
		}
		savedID, err := repo.SaveGroup(r.Context(), admin.SaveGroupInput{
			ID:          id,
			Name:        name,
			Description: description,
		}, memberIDs)
		if err != nil {
			if id > 0 {
				data.Error = "Failed to update group (name may already exist)."
			} else {
				data.Error = "Failed to create group (name may already exist)."
			}
			data.EditID = id
			s.recordAudit(r, audit.Event{
				Category:   audit.CategorySecurity,
				Action:     saveAction,
				ObjectType: audit.ObjectGroup,
				ObjectID:   strconv.FormatInt(id, 10),
				ObjectName: name,
				Status:     audit.StatusFailure,
				Details:    map[string]any{"error": audit.SanitizeErrorMessage(err.Error())},
			})
		} else {
			if id > 0 {
				data.Message = "Group updated."
			} else {
				data.Message = "Group created."
			}
			data.EditID = 0
			objectID := strconv.FormatInt(id, 10)
			if savedID > 0 {
				objectID = strconv.FormatInt(savedID, 10)
			}
			s.recordAudit(r, audit.Event{
				Category:   audit.CategorySecurity,
				Action:     saveAction,
				ObjectType: audit.ObjectGroup,
				ObjectID:   objectID,
				ObjectName: name,
				Status:     audit.StatusSuccess,
			})
		}
	}

	s.loadAdminGroupsPage(r, repo, data)
	s.renderAdminGroups(w, *data)
}

func (s *Server) loadAdminGroupsPage(r *http.Request, repo *admin.Repository, data *adminGroupsPageData) {
	allUsers, err := repo.ListAllUsers(r.Context())
	if err != nil {
		data.Error = "Unable to connect to the application PostgreSQL database. Check config.php or JRS_DB_* environment variables."
		return
	}
	data.AllUsers = allUsers

	groups, err := repo.ListGroups(r.Context())
	if err != nil {
		data.Error = "Unable to connect to the application PostgreSQL database. Check config.php or JRS_DB_* environment variables."
		return
	}
	data.Groups = groups

	if data.EditID > 0 {
		for i := range groups {
			if groups[i].ID == data.EditID {
				g := groups[i]
				data.EditGroup = &g
				break
			}
		}
		ids, err := repo.GroupMemberIDs(r.Context(), data.EditID)
		if err == nil {
			data.EditMembers = int64Set(ids)
		}
	}
}

func (s *Server) renderAdminGroups(w http.ResponseWriter, data adminGroupsPageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := adminGroupsTemplate.Execute(w, data); err != nil {
		s.logger.Error("render admin groups", "error", err)
	}
}
