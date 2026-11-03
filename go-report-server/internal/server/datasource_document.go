package server

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/acugis/acugis-report-server-go/collectionfile"
	"github.com/acugis/acugis-report-server-go/internal/auth"
	"github.com/acugis/acugis-report-server-go/internal/datasources"
	"github.com/acugis/acugis-report-server-go/jsonurl"
)

func (s *Server) datasourceFilesDir() string {
	if s != nil {
		if v := strings.TrimSpace(s.cfg.Paths.DatasourceFilesDir); v != "" {
			return v
		}
	}
	return datasources.DefaultDatasourceFilesDir()
}

func parseDatasourceRequest(r *http.Request) error {
	if err := r.ParseMultipartForm(int64(collectionfile.MaxBytes) + (1 << 20)); err != nil {
		return r.ParseForm()
	}
	return nil
}

func documentFieldsFromForm(r *http.Request, ds datasources.Datasource) (datasources.Datasource, []string) {
	var errs []string
	ds.Host = ""
	ds.Port = 0
	ds.DBName = ""
	ds.Username = ""
	ds.Password = ""
	ds.SchemaName = ""
	ds.AuthType = datasources.AuthPassword
	ds.PublicKeyResourceUUID = ""
	ds.PrivateKeyResourceUUID = ""
	ds.Options = map[string]string{}
	if ds.Name == "" {
		errs = append(errs, "Name is required.")
	}
	switch ds.DBType {
	case "csv":
		delim := strings.TrimSpace(r.FormValue("delimiter"))
		if delim != "" {
			if _, _, err := collectionfile.ParseDelimiter(delim); err != nil {
				errs = append(errs, err.Error())
			} else {
				ds.Options[datasources.OptionDelimiter] = delim
			}
		}
	case "json":
		source := strings.ToLower(strings.TrimSpace(r.FormValue("json_source")))
		if source == "" {
			source = datasources.JSONSourceFile
		}
		if source != datasources.JSONSourceFile && source != datasources.JSONSourceURL {
			errs = append(errs, "JSON source must be file or url.")
			source = datasources.JSONSourceFile
		}
		ds.Options[datasources.OptionJSONSource] = source
		if source == datasources.JSONSourceURL {
			ds.Options[datasources.OptionJSONURL] = strings.TrimSpace(r.FormValue("json_url"))
		}
		if c := strings.TrimSpace(r.FormValue("collection")); c != "" {
			ds.Options[datasources.OptionCollection] = c
		}
	case "excel":
		if sheet := strings.TrimSpace(r.FormValue("sheet")); sheet != "" {
			ds.Options[datasources.OptionSheet] = sheet
		}
	case "xml":
		if c := strings.TrimSpace(r.FormValue("collection")); c != "" {
			ds.Options[datasources.OptionCollection] = c
		}
	}
	return ds, errs
}

func (s *Server) applyDocumentForm(r *http.Request, store datasources.Store, ds *datasources.Datasource) error {
	var existing *datasources.Datasource
	if store != nil && strings.TrimSpace(ds.Name) != "" {
		if row, err := store.Find(r.Context(), ds.Name); err == nil {
			existing = row
		}
	}
	name, body, err := documentUpload(r)
	if err != nil {
		return err
	}
	return datasources.PrepareDocument(ds, datasources.DocumentInput{
		Root:        s.datasourceFilesDir(),
		Existing:    existing,
		UploadName:  name,
		Upload:      body,
		ClaimedPath: r.FormValue("document_resource"),
	})
}

func documentUpload(r *http.Request) (string, []byte, error) {
	file, hdr, err := r.FormFile("document")
	if err != nil {
		if errors.Is(err, http.ErrMissingFile) {
			return "", nil, nil
		}
		return "", nil, err
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, int64(collectionfile.MaxBytes)+1))
	if err != nil {
		return "", nil, err
	}
	filename := ""
	if hdr != nil {
		filename = hdr.Filename
	}
	return filename, body, nil
}

func (s *Server) handleAPIDocumentDiscover(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if err := s.requireServiceAdmin(r, user); err != nil {
		s.writeAuthzJSON(w, err)
		return
	}
	if err := parseDatasourceRequest(r); err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "message": "Failed to parse form."})
		return
	}
	dbType := datasources.NormalizeDBType(r.FormValue("db_type"))
	if !datasources.IsDocumentType(dbType) {
		s.writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "message": "Document type is required."})
		return
	}
	if dbType == "json" && strings.EqualFold(r.FormValue("json_source"), datasources.JSONSourceURL) {
		raw := strings.TrimSpace(r.FormValue("json_url"))
		body, err := jsonurl.Get(r.Context(), raw)
		if err != nil {
			s.writeJSON(w, http.StatusOK, map[string]any{"ok": false, "message": err.Error()})
			return
		}
		found, err := datasources.DiscoverBytes(dbType, "document.json", body)
		if err != nil {
			s.writeJSON(w, http.StatusOK, map[string]any{"ok": false, "message": err.Error()})
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "sheets": found.Sheets, "collections": found.Collections})
		return
	}
	name, body, err := documentUpload(r)
	if err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "message": "invalid document upload"})
		return
	}
	var found datasources.Discovery
	if len(body) > 0 {
		found, err = datasources.DiscoverBytes(dbType, name, body)
	} else if store := s.dsStore(); store != nil {
		row, ferr := store.Find(r.Context(), strings.TrimSpace(r.FormValue("name")))
		if ferr != nil || row == nil {
			err = ferr
			if err == nil {
				err = datasources.ErrNotFound
			}
		} else {
			found, err = datasources.DiscoverStored(s.datasourceFilesDir(), *row)
		}
	} else {
		err = datasources.ErrNotFound
	}
	if err != nil {
		s.writeJSON(w, http.StatusOK, map[string]any{"ok": false, "message": err.Error()})
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "sheets": found.Sheets, "collections": found.Collections})
}

func (s *Server) handleAPIDocumentTest(w http.ResponseWriter, r *http.Request, ds datasources.Datasource) {
	start := time.Now()
	name, body, err := documentUpload(r)
	if err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "message": "invalid document upload"})
		return
	}
	var msg string
	if len(body) > 0 || ds.IsJSONURL() {
		msg, err = datasources.TestDocumentBytes(r.Context(), ds, name, body)
	} else if store := s.dsStore(); store != nil {
		if row, ferr := store.Find(r.Context(), ds.Name); ferr == nil && row != nil {
			merged := *row
			if ds.Options != nil {
				if merged.Options == nil {
					merged.Options = map[string]string{}
				}
				for k, v := range ds.Options {
					if strings.TrimSpace(v) != "" {
						merged.Options[k] = v
					}
				}
			}
			msg, err = datasources.TestDocumentStored(r.Context(), s.datasourceFilesDir(), merged)
		} else {
			err = datasources.ErrNotFound
		}
	} else {
		err = datasources.ErrNotFound
	}
	if err != nil {
		text := err.Error()
		if errors.Is(err, datasources.ErrNotFound) {
			text = "a document file is required"
		}
		s.recordDatasourceTest(r, ds, false, start, text)
		s.writeJSON(w, http.StatusOK, map[string]any{"ok": false, "message": text})
		return
	}
	s.recordDatasourceTest(r, ds, true, start, "")
	s.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": msg})
}

func (s *Server) documentLogicalForID(r *http.Request, store datasources.Store, id int64) string {
	if store == nil || id <= 0 {
		return ""
	}
	rows, err := store.ListEnabled(r.Context())
	if err != nil {
		return ""
	}
	for _, row := range rows {
		if row.ID != id || !row.IsDocument() {
			continue
		}
		return strings.TrimSpace(row.Option(datasources.OptionResourceFile))
	}
	return ""
}
