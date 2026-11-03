package server

import (
	"context"
	"errors"
	"strings"

	"github.com/acugis/acugis-report-server-go/internal/folders"
	"github.com/acugis/acugis-report-server-go/internal/jasper"
	"github.com/acugis/acugis-report-server-go/internal/reports"
	"github.com/acugis/acugis-report-server-go/resourcefs"
)

// A Designer report owns an isolated resource directory:
//
//	folder report:  {catalog folder}/{report key}   DavidG/testingone
//	Home report:    {report key}                    testingone
//
// A legacy report shares the catalog folder directory (DavidG, or no file in
// Home). Execution uses whichever valid sidecar is already stored. It writes
// the catalog folder only when the sidecar is missing or malformed.
// Moving a Designer report relocates that directory so the sidecar matches
// the files. Moving a legacy report still points the sidecar at the new folder.

func (s *Server) lookupReportItem(ctx context.Context, reportKey string) (int64, bool) {
	if s.testReportFolder != nil {
		return s.testReportFolder(reportKey)
	}
	if s.deps.Folders == nil || s.deps.Folders.Repo == nil {
		return 0, false
	}
	it, err := s.deps.Folders.Repo.GetItemByKey(ctx, folders.ContentTypeReport, reportKey)
	if err != nil || it == nil {
		return 0, false
	}
	return it.FolderID, true
}

func (s *Server) slashForFolder(ctx context.Context, folderID int64) string {
	if s.testSlashFolder != nil {
		slash, err := s.testSlashFolder(folderID)
		if err != nil {
			s.logger.Warn("report resource base path", "folder", folderID, "error", err)
			return ""
		}
		return slash
	}
	if s.deps.Folders == nil || folderID <= 0 {
		return ""
	}
	slash, err := s.deps.Folders.SlashFolderPath(ctx, folderID)
	if err != nil {
		s.logger.Warn("report resource base path", "folder", folderID, "error", err)
		return ""
	}
	return slash
}

// resourceBaseForExecute returns the base to pass as _acugisResourceBase.
// A readable sidecar is preserved. Missing or malformed metadata falls back
// to the catalog folder, which is the historical sync behavior.
func (s *Server) resourceBaseForExecute(reportKey, catalogSlash string) string {
	catalog, _ := reports.NormalizeResourceBase(catalogSlash)
	if s.deps.Reports == nil {
		return catalog
	}
	stored, err := s.deps.Reports.ReadResourceBase(reportKey)
	if err == nil && stored != "" {
		return stored
	}
	if err != nil && !errors.Is(err, reports.ErrNotFound) {
		s.logger.Warn("malformed report resource base", "report", reportKey, "error", err)
	}
	if err := s.deps.Reports.SaveResourceBase(reportKey, catalogSlash); err != nil {
		s.logger.Warn("save report resource base", "report", reportKey, "folder", catalog, "error", err)
	}
	return catalog
}

// refreshPublishedResourceBase keeps a Designer per-report base. Legacy and
// missing bases are still written from the catalog folder.
func (s *Server) refreshPublishedResourceBase(ctx context.Context, reportKey string, folderID int64) {
	slash := s.slashForFolder(ctx, folderID)
	if s.preserveDesignerResourceBase(reportKey, slash) {
		return
	}
	s.syncReportResourceBase(ctx, reportKey, folderID)
}

func (s *Server) preserveDesignerResourceBase(reportKey, catalogSlash string) bool {
	if s.deps.Reports == nil {
		return false
	}
	stored, err := s.deps.Reports.ReadResourceBase(reportKey)
	if err != nil || stored == "" {
		return false
	}
	catalog, normErr := reports.NormalizeResourceBase(catalogSlash)
	if normErr == nil && catalog != "" && stored == catalog {
		return false
	}
	return isDesignerResourceBase(stored, reportKey)
}

// applyResourceBaseOnMove keeps the sidecar consistent with the resource files
// after a catalog move. A Designer directory is renamed to the new per-report
// base. A legacy base is pointed at the destination folder and its shared
// directory is not moved.
func (s *Server) applyResourceBaseOnMove(ctx context.Context, reportKey string, oldFolderID, newFolderID int64) {
	if s.deps.Reports == nil {
		return
	}
	oldSlash := s.slashForFolder(ctx, oldFolderID)
	newSlash := s.slashForFolder(ctx, newFolderID)
	oldCatalog, _ := reports.NormalizeResourceBase(oldSlash)
	stored, err := s.deps.Reports.ReadResourceBase(reportKey)
	if err == nil && stored != "" && isDesignerResourceBase(stored, reportKey) && stored != oldCatalog {
		newBase, berr := designerResourceBase(newSlash, reportKey)
		if berr != nil || newBase == "" {
			s.logger.Warn("designer resource base", "report", reportKey, "error", berr)
			return
		}
		if newBase == stored {
			return
		}
		if rerr := s.relocateDesignerResources(stored, newBase); rerr != nil {
			s.logger.Warn("move report resource base", "report", reportKey, "from", stored, "to", newBase, "error", rerr)
			return
		}
		if werr := s.deps.Reports.SaveResourceBase(reportKey, newBase); werr != nil {
			s.logger.Warn("save report resource base", "report", reportKey, "folder", newBase, "error", werr)
		}
		return
	}
	if err := s.deps.Reports.SaveResourceBase(reportKey, newSlash); err != nil {
		s.logger.Warn("save report resource base", "report", reportKey, "folder", newSlash, "error", err)
	}
}

func (s *Server) relocateDesignerResources(from, to string) error {
	if from == to {
		return nil
	}
	dir := strings.TrimSpace(s.cfg.Paths.ResourcePathsDir)
	if dir == "" || s.deps.Reports == nil || s.deps.Reports.JRXMLDir() == "" {
		return resourcefs.ErrUnavailable
	}
	return resourcefs.NewStore(s.deps.Reports.JRXMLDir(), dir).RelocateIsolated(from, to)
}

// isDesignerResourceBase reports whether base is an isolated per-report
// directory: the report key itself (Home) or a path ending in /{key}.
// A legacy base that is exactly the current catalog folder is not this shape
// unless that folder path is also the key, which the caller rejects by
// comparing the stored base with the catalog folder first.
func isDesignerResourceBase(base, reportKey string) bool {
	base, err := reports.NormalizeResourceBase(base)
	if err != nil || base == "" || !jasper.IsValidReportKey(reportKey) {
		return false
	}
	if base == reportKey {
		return true
	}
	return strings.HasSuffix(base, "/"+reportKey)
}
