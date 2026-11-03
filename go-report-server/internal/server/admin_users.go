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
	"github.com/acugis/acugis-report-server-go/internal/extid"
)

//go:embed admin_users.html
var adminUsersHTML string

type adminUsersPageData struct {
	materialChromeData
	UsersURL           string
	GroupsURL          string
	LogoutURL          string
	Message            string
	Error              string
	EditID             int64
	EditUser           *admin.UserRow
	EditGroups         map[int64]bool
	Groups             []admin.GroupRef
	Users              []admin.UserRow
	EditIdentities     []extIdentityView
	UserIdentitySummary map[int64]string
}

type extIdentityView struct {
	ProviderType string
	ProviderName string
	Email        string
	Subject      string
}

var adminUsersTemplate = template.Must(template.New("admin_users").Funcs(template.FuncMap{
	"containsInt": func(m map[int64]bool, id int64) bool {
		return m != nil && m[id]
	},
}).Parse(adminUsersHTML))

func (s *Server) adminRepo() *admin.Repository {
	return s.deps.Admin
}

func (s *Server) handleAdminUsersPage(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if err := s.requirePlatformAdmin(r, user); err != nil {
		s.writeAuthzHTML(w, r, err)
		return
	}

	data := adminUsersPageData{
		materialChromeData: s.materialChrome("Users"),
		UsersURL:           s.goURL("/admin/users"),
		GroupsURL:          s.goURL("/admin/groups"),
		LogoutURL:          s.goURL("/logout"),
	}

	repo := s.adminRepo()
	if repo == nil || !repo.Configured() {
		data.Error = "Unable to connect to the application PostgreSQL database. Check config.php or JRS_DB_* environment variables."
		s.renderAdminUsers(w, data)
		return
	}

	editID, _ := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("id")), 10, 64)
	data.EditID = editID

	if r.Method == http.MethodPost {
		s.handleAdminUsersPOST(w, r, user, repo, &data)
		return
	}

	s.loadAdminUsersPage(r, repo, &data)
	s.renderAdminUsers(w, data)
}

func (s *Server) handleAdminUsersPOST(w http.ResponseWriter, r *http.Request, current *auth.User, repo *admin.Repository, data *adminUsersPageData) {
	if err := r.ParseForm(); err != nil {
		data.Error = "Failed to parse form."
		s.loadAdminUsersPage(r, repo, data)
		s.renderAdminUsers(w, *data)
		return
	}

	action := strings.TrimSpace(r.FormValue("action"))
	id, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("id")), 10, 64)
	currentID := int64(0)
	if current != nil {
		currentID = current.ID
	}

	if action == "delete" && id > 0 {
		if err := repo.DeleteUser(r.Context(), id, currentID); err != nil {
			if errors.Is(err, admin.ErrSelfDelete) {
				data.Error = "You cannot delete your own account."
			} else {
				data.Error = "Failed to delete user."
			}
			data.EditID = id
			s.recordAudit(r, audit.Event{
				Category:   audit.CategorySecurity,
				Action:     audit.ActionDelete,
				ObjectType: audit.ObjectUser,
				ObjectID:   strconv.FormatInt(id, 10),
				Status:     audit.StatusFailure,
				Details:    map[string]any{"error": audit.SanitizeErrorMessage(err.Error())},
			})
		} else {
			data.Message = "User deleted."
			data.EditID = 0
			s.recordAudit(r, audit.Event{
				Category:   audit.CategorySecurity,
				Action:     audit.ActionDelete,
				ObjectType: audit.ObjectUser,
				ObjectID:   strconv.FormatInt(id, 10),
				Status:     audit.StatusSuccess,
			})
		}
		s.loadAdminUsersPage(r, repo, data)
		s.renderAdminUsers(w, *data)
		return
	}

	username := strings.TrimSpace(r.FormValue("username"))
	email := strings.TrimSpace(r.FormValue("email"))
	password := r.FormValue("password")
	isAdmin := r.FormValue("is_admin") != ""
	enabled := r.FormValue("enabled") != ""
	groupIDs := parseInt64Slice(r.Form["groups"])

	if username == "" {
		data.Error = "Username is required."
		data.EditID = id
	} else if id <= 0 && password == "" {
		data.Error = "Password is required for new users."
		data.EditID = id
	} else {
		saveAction := audit.ActionCreate
		if id > 0 {
			saveAction = audit.ActionUpdate
		}
		savedID, err := repo.SaveUser(r.Context(), admin.SaveUserInput{
			ID:       id,
			Username: username,
			Email:    email,
			Password: password,
			IsAdmin:  isAdmin,
			Enabled:  enabled,
		}, groupIDs)
		if err != nil {
			if errors.Is(err, admin.ErrInvalidInput) {
				if id <= 0 && password == "" {
					data.Error = "Password is required for new users."
				} else {
					data.Error = "Username is required."
				}
			} else {
				if id > 0 {
					data.Error = "Failed to update user (username may already exist)."
				} else {
					data.Error = "Failed to create user (username may already exist)."
				}
			}
			data.EditID = id
			s.recordAudit(r, audit.Event{
				Category:   audit.CategorySecurity,
				Action:     saveAction,
				ObjectType: audit.ObjectUser,
				ObjectID:   strconv.FormatInt(id, 10),
				ObjectName: username,
				Status:     audit.StatusFailure,
				Details:    map[string]any{"error": audit.SanitizeErrorMessage(err.Error())},
			})
		} else {
			if id > 0 {
				data.Message = "User updated."
			} else {
				data.Message = "User created."
			}
			data.EditID = 0
			objectID := strconv.FormatInt(id, 10)
			if savedID > 0 {
				objectID = strconv.FormatInt(savedID, 10)
			}
			s.recordAudit(r, audit.Event{
				Category:   audit.CategorySecurity,
				Action:     saveAction,
				ObjectType: audit.ObjectUser,
				ObjectID:   objectID,
				ObjectName: username,
				Status:     audit.StatusSuccess,
			})
		}
	}

	s.loadAdminUsersPage(r, repo, data)
	s.renderAdminUsers(w, *data)
}

func (s *Server) loadAdminUsersPage(r *http.Request, repo *admin.Repository, data *adminUsersPageData) {
	groups, err := repo.ListAllGroups(r.Context())
	if err != nil {
		data.Error = "Unable to connect to the application PostgreSQL database. Check config.php or JRS_DB_* environment variables."
		return
	}
	data.Groups = groups

	users, err := repo.ListUsers(r.Context())
	if err != nil {
		data.Error = "Unable to connect to the application PostgreSQL database. Check config.php or JRS_DB_* environment variables."
		return
	}
	data.Users = users
	data.UserIdentitySummary = map[int64]string{}

	if s.deps.OIDC != nil {
		if ext := s.deps.OIDC.ExtRepository(); ext != nil && ext.Configured() {
			ids := make([]int64, 0, len(users))
			for _, u := range users {
				ids = append(ids, u.ID)
			}
			byUser, lerr := ext.ListByUserIDs(r.Context(), ids)
			if lerr == nil {
				for uid, list := range byUser {
					parts := make([]string, 0, len(list))
					for _, id := range list {
						label := strings.ToUpper(id.ProviderType)
						if id.Email != "" {
							parts = append(parts, label+": "+id.Email)
						} else {
							parts = append(parts, label)
						}
					}
					data.UserIdentitySummary[uid] = strings.Join(parts, "; ")
				}
			}
			if data.EditID > 0 {
				idents, ierr := ext.ListByUserID(r.Context(), data.EditID)
				if ierr == nil {
					for _, id := range idents {
						providerName := id.ProviderID
						if cfg, cerr := s.deps.OIDC.LoadSettings(r.Context()); cerr == nil && cfg.ProviderName != "" && id.ProviderType == extid.ProviderTypeOIDC {
							providerName = cfg.ProviderName
						}
						if s.deps.SAML != nil {
							if cfg, cerr := s.deps.SAML.LoadSettings(r.Context()); cerr == nil && cfg.ProviderName != "" && id.ProviderType == extid.ProviderTypeSAML {
								providerName = cfg.ProviderName
							}
						}
						data.EditIdentities = append(data.EditIdentities, extIdentityView{
							ProviderType: strings.ToUpper(id.ProviderType),
							ProviderName: providerName,
							Email:        id.Email,
							Subject:      id.Subject,
						})
					}
				}
			}
		}
	}

	if data.EditID > 0 {
		for i := range users {
			if users[i].ID == data.EditID {
				u := users[i]
				data.EditUser = &u
				break
			}
		}
		ids, err := repo.UserGroupIDs(r.Context(), data.EditID)
		if err == nil {
			data.EditGroups = int64Set(ids)
		}
	}
}

func (s *Server) renderAdminUsers(w http.ResponseWriter, data adminUsersPageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := adminUsersTemplate.Execute(w, data); err != nil {
		s.logger.Error("render admin users", "error", err)
	}
}

func parseInt64Slice(values []string) []int64 {
	out := make([]int64, 0, len(values))
	for _, v := range values {
		id, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		if err != nil || id <= 0 {
			continue
		}
		out = append(out, id)
	}
	return out
}

func int64Set(ids []int64) map[int64]bool {
	m := make(map[int64]bool, len(ids))
	for _, id := range ids {
		m[id] = true
	}
	return m
}
