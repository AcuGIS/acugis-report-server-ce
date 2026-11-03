package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/acugis/acugis-report-server-go/internal/arcgis"
	"github.com/acugis/acugis-report-server-go/internal/datasources"
)

const (
	arcgisNameMax = 80
	arcgisURLMax  = 500
)

func (s *Server) arcgisDatasourcesEnabled() bool {
	return s != nil && s.deps.ArcGIS != nil
}

func (s *Server) listArcGISDatasources(ctx context.Context) []datasources.Datasource {
	if s == nil || s.deps.ArcGIS == nil {
		return nil
	}
	rows, err := s.deps.ArcGIS.ListEnabled(ctx)
	if err != nil {
		return nil
	}
	out := make([]datasources.Datasource, 0, len(rows))
	for _, row := range rows {
		if strings.TrimSpace(row.Name) == "" {
			continue
		}
		out = append(out, arcGISConnectionToDatasource(row))
	}
	return out
}

func (s *Server) arcgisDatasourceNames(ctx context.Context) []string {
	rows := s.listArcGISDatasources(ctx)
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.Name)
	}
	return out
}

func (s *Server) arcgisDatasourceExists(ctx context.Context, name string) bool {
	name = strings.TrimSpace(name)
	if name == "" || s == nil || s.deps.ArcGIS == nil {
		return false
	}
	_, err := s.deps.ArcGIS.FindArcGIS(ctx, name)
	return err == nil
}

func (s *Server) findArcGISDatasource(ctx context.Context, name string) (datasources.Datasource, bool) {
	name = strings.TrimSpace(name)
	if name == "" || s == nil || s.deps.ArcGIS == nil {
		return datasources.Datasource{}, false
	}
	cred, err := s.deps.ArcGIS.FindArcGIS(ctx, name)
	if err != nil {
		return datasources.Datasource{}, false
	}
	rows, listErr := s.deps.ArcGIS.ListEnabled(ctx)
	id := int64(0)
	if listErr == nil {
		for _, row := range rows {
			if row.Name == cred.Name {
				id = row.ID
				break
			}
		}
	}
	return arcGISConnectionToDatasource(arcgis.Connection{
		ID:       id,
		Name:     cred.Name,
		URL:      cred.URL,
		Username: cred.Username,
		Password: cred.Password,
	}), true
}

func arcGISConnectionToDatasource(c arcgis.Connection) datasources.Datasource {
	serviceURL := strings.TrimRight(strings.TrimSpace(c.URL), "/")
	return datasources.Datasource{
		ID:       c.ID,
		Name:     strings.TrimSpace(c.Name),
		DBType:   "arcgis",
		Host:     serviceURL,
		DBName:   serviceURL,
		Username: strings.TrimSpace(c.Username),
		Password: c.Password,
		AuthType: datasources.AuthPassword,
	}
}

func (s *Server) saveArcGISDatasource(r *http.Request, ds datasources.Datasource) error {
	if s == nil || s.deps.ArcGIS == nil {
		return errors.New("ArcGIS connection catalog is unavailable")
	}
	if !s.deps.ArcGIS.TableAvailable(r.Context()) {
		return errors.New("Missing public.jasper_arcgis. Load database/schema.sql or database/migrate_arcgis.sql into the application database.")
	}
	name := strings.TrimSpace(ds.Name)
	if name == "" {
		return errors.New("Name and Service URL are required.")
	}
	if strings.Contains(name, ".") {
		return errors.New("Datasource name must not contain a period.")
	}
	if len(name) > arcgisNameMax {
		return fmt.Errorf("Datasource name must be %d characters or fewer.", arcgisNameMax)
	}
	serviceURL, err := arcgis.NormalizeServiceURL(ds.Host)
	if err != nil {
		return err
	}
	if len(serviceURL) > arcgisURLMax {
		return fmt.Errorf("Service URL must be %d characters or fewer.", arcgisURLMax)
	}
	username := strings.TrimSpace(ds.Username)
	password := ds.Password
	if username == "" {
		password = ""
	} else if strings.TrimSpace(password) == "" {
		if existing, ferr := s.deps.ArcGIS.FindArcGIS(r.Context(), name); ferr == nil {
			password = existing.Password
		}
	}
	if store := s.dsStore(); store != nil {
		if existing, ferr := store.Find(r.Context(), name); ferr == nil && existing != nil && !existing.IsArcGIS() {
			return fmt.Errorf("A datasource named %q already exists.", name)
		}
	}
	if s.geoServerDatasourceExists(r.Context(), name) {
		return fmt.Errorf("A GeoServer datasource named %q already exists.", name)
	}
	if err := s.deps.ArcGIS.Upsert(r.Context(), arcgis.Connection{
		Name:     name,
		URL:      serviceURL,
		Username: username,
		Password: password,
	}); err != nil {
		if errors.Is(err, arcgis.ErrInvalidInput) {
			return errors.New("Name and Service URL are required, and the name must not contain a period.")
		}
		return errors.New("Failed to save ArcGIS Feature Server datasource.")
	}
	if err := s.writeArcGISProps(r, s.deps.ArcGIS); err != nil {
		s.logger.Warn("arcgis properties", "error", err)
		return errArcGISProps
	}
	return nil
}

func (s *Server) deleteArcGISDatasource(r *http.Request, id int64) error {
	if s == nil || s.deps.ArcGIS == nil {
		return errors.New("ArcGIS connection catalog is unavailable")
	}
	if err := s.deps.ArcGIS.DeleteByID(r.Context(), id); err != nil {
		return errors.New("Failed to delete datasource.")
	}
	if err := s.writeArcGISProps(r, s.deps.ArcGIS); err != nil {
		s.logger.Warn("arcgis properties", "error", err)
		return errArcGISProps
	}
	return nil
}

func (s *Server) testArcGISDatasource(ctx context.Context, ds datasources.Datasource) (string, error) {
	username := strings.TrimSpace(ds.Username)
	password := ds.Password
	if username != "" && strings.TrimSpace(password) == "" && strings.TrimSpace(ds.Name) != "" && s != nil && s.deps.ArcGIS != nil {
		if existing, err := s.deps.ArcGIS.FindArcGIS(ctx, ds.Name); err == nil {
			password = existing.Password
		}
	}
	if username == "" {
		password = ""
	}
	return arcgis.TestConnection(ctx, arcgis.Credentials{
		Name:     strings.TrimSpace(ds.Name),
		URL:      ds.Host,
		Username: username,
		Password: password,
	})
}

func (s *Server) handleAPIArcGISTest(w http.ResponseWriter, r *http.Request, ds datasources.Datasource) {
	if !s.arcgisDatasourcesEnabled() {
		s.writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "message": "ArcGIS Feature Server is not available."})
		return
	}
	start := time.Now()
	msg, err := s.testArcGISDatasource(r.Context(), ds)
	if err != nil {
		s.recordDatasourceTest(r, ds, false, start, err.Error())
		s.writeJSON(w, http.StatusOK, map[string]any{"ok": false, "message": err.Error()})
		return
	}
	if strings.TrimSpace(msg) == "" {
		msg = "Connected"
	}
	s.recordDatasourceTest(r, ds, true, start, "")
	s.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": msg})
}

func (s *Server) writeArcGISProps(r *http.Request, store arcgis.Store) error {
	rows, err := store.ListEnabled(r.Context())
	if err != nil {
		return err
	}
	path := strings.TrimSpace(s.cfg.Paths.ArcGISConnectionsPath)
	return arcgis.WriteConnectionsFile(path, rows)
}
