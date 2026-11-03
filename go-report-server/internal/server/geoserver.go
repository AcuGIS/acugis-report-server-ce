package server

import (
	_ "embed"
	"html/template"
	"net/http"
	"strconv"
	"strings"

	"github.com/acugis/acugis-report-server-go/internal/auth"
	"github.com/acugis/acugis-report-server-go/internal/geoserver"
)

//go:embed geoserver_connections.html
var geoserverConnectionsHTML string

type gsRowView struct {
	ID       int64
	Name     string
	URL      string
	Username string
}

type geoserverPageData struct {
	materialChromeData
	PropsPath   string
	Datasources string
	Errors      []string
	Message     string
	Rows        []gsRowView
}

var geoserverTemplate = template.Must(template.New("geoserver").Parse(geoserverConnectionsHTML))

func (s *Server) gsStore() geoserver.Store {
	return s.deps.GeoServer
}

func (s *Server) handleGeoServerConnectionsPage(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if err := s.requireServiceAdmin(r, user); err != nil {
		s.writeAuthzHTML(w, r, err)
		return
	}

	data := geoserverPageData{
		materialChromeData: s.materialChrome("GeoServer Connections"),
		PropsPath:          strings.TrimSpace(s.cfg.Paths.GeoServerConnectionsPath),
		Datasources:        s.goURL("/reports/datasources"),
	}

	store := s.gsStore()
	if store == nil {
		data.Errors = append(data.Errors, "Unable to connect to the application PostgreSQL database. Check config.php or JRS_DB_* environment variables.")
		s.renderGeoServer(w, data)
		return
	}
	if !store.TableAvailable(r.Context()) {
		data.Errors = append(data.Errors, "Missing public.jasper_geoserver. Load database/schema.sql or database/migrate_geoserver.sql into the application database.")
		s.renderGeoServer(w, data)
		return
	}

	if r.Method == http.MethodPost {
		s.handleGeoServerPOST(w, r, store, &data)
		return
	}

	s.loadGeoServerRows(r, store, &data)
	s.renderGeoServer(w, data)
}

func (s *Server) handleGeoServerPOST(w http.ResponseWriter, r *http.Request, store geoserver.Store, data *geoserverPageData) {
	if err := r.ParseForm(); err != nil {
		data.Errors = append(data.Errors, "Failed to parse form.")
		s.renderGeoServer(w, *data)
		return
	}

	action := strings.TrimSpace(r.FormValue("action"))
	switch action {
	case "delete":
		id, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("id")), 10, 64)
		if id > 0 {
			if err := store.DeleteByID(r.Context(), id); err != nil {
				data.Errors = append(data.Errors, "Failed to delete GeoServer connection.")
			} else {
				if err := s.writeGeoServerProps(r, store); err != nil {
					data.Errors = append(data.Errors, "Deleted, but failed to regenerate properties file.")
					s.logger.Warn("geoserver properties", "error", err)
				} else {
					data.Message = "GeoServer connection deleted. Properties file regenerated."
				}
			}
		}
	case "regenerate":
		if err := s.writeGeoServerProps(r, store); err != nil {
			data.Errors = append(data.Errors, "Failed to write geoserver-connections.properties. Check geoserver_connections_path / permissions.")
			s.logger.Warn("geoserver properties", "error", err)
		} else {
			data.Message = "GeoServer connections properties file regenerated for the Report Server WAR."
		}
	default:
		conn := geoserver.Connection{
			Name:     strings.TrimSpace(r.FormValue("name")),
			URL:      strings.TrimSpace(r.FormValue("url")),
			Username: strings.TrimSpace(r.FormValue("username")),
			Password: r.FormValue("password"),
		}
		if conn.Name == "" || conn.URL == "" {
			data.Errors = append(data.Errors, "Name and GeoServer base URL are required.")
		} else if strings.Contains(conn.Name, ".") {
			data.Errors = append(data.Errors, "Connection name must not contain a period (used as the properties key separator).")
		} else if err := store.Upsert(r.Context(), conn); err != nil {
			data.Errors = append(data.Errors, "Failed to save GeoServer connection.")
			s.logger.Warn("save geoserver connection", "error", err)
		} else if err := s.writeGeoServerProps(r, store); err != nil {
			data.Errors = append(data.Errors, "Saved to database, but failed to write geoserver-connections.properties. Check path / permissions.")
			s.logger.Warn("geoserver properties", "error", err)
		} else {
			data.Message = "GeoServer connection saved. Use the same Name as the Designer Connection Manager entry."
		}
	}

	s.loadGeoServerRows(r, store, data)
	s.renderGeoServer(w, *data)
}

func (s *Server) writeGeoServerProps(r *http.Request, store geoserver.Store) error {
	rows, err := store.ListEnabled(r.Context())
	if err != nil {
		return err
	}
	path := strings.TrimSpace(s.cfg.Paths.GeoServerConnectionsPath)
	return geoserver.WriteConnectionsFile(path, rows)
}

func (s *Server) loadGeoServerRows(r *http.Request, store geoserver.Store, data *geoserverPageData) {
	rows, err := store.ListEnabled(r.Context())
	if err != nil {
		if len(data.Errors) == 0 {
			data.Errors = append(data.Errors, "Unable to connect to the application PostgreSQL database. Check config.php or JRS_DB_* environment variables.")
		}
		return
	}
	data.Rows = make([]gsRowView, 0, len(rows))
	for _, row := range rows {
		user := row.Username
		if user == "" {
			user = "—"
		}
		data.Rows = append(data.Rows, gsRowView{
			ID:       row.ID,
			Name:     row.Name,
			URL:      row.URL,
			Username: user,
		})
	}
}

func (s *Server) renderGeoServer(w http.ResponseWriter, data geoserverPageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := geoserverTemplate.Execute(w, data); err != nil {
		s.logger.Error("render geoserver connections", "error", err)
	}
}
