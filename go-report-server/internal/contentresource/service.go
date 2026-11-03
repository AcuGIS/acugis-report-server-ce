package contentresource

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/google/uuid"

	"github.com/acugis/acugis-report-server-go/internal/folders"
)

// FolderPlacer places content_items membership for resources.
type FolderPlacer interface {
	EnsureResourceInFolder(ctx context.Context, folderID int64, resourceUUID, displayName string, createdBy int64) error
	MoveResource(ctx context.Context, folderID int64, resourceUUID, displayName string, createdBy int64) error
	RemoveResource(ctx context.Context, resourceUUID string) error
	PathString(ctx context.Context, folderID int64) (string, error)
	// SlashFolderPath returns "/finance/images" style path (empty for Home).
	SlashFolderPath(ctx context.Context, folderID int64) (string, error)
}

// Service orchestrates DB + filesystem + folder membership for resources.
type Service struct {
	Repo       *Repository
	Files      *FileStore
	Thumbnails *ThumbnailStore
	Paths      *PathStore // Image Resources on disk for Jasper (path = JRXML path)
	Folders    FolderPlacer
}

// NewService constructs a content resource service.
func NewService(repo *Repository, files *FileStore, placer FolderPlacer) *Service {
	return &Service{Repo: repo, Files: files, Folders: placer}
}

// Configured reports whether create/list can run.
func (s *Service) Configured() bool {
	return s != nil && s.Repo != nil && s.Repo.Configured() && s.Files != nil && s.Files.Dir != ""
}

// Create writes bytes to disk, inserts the DB row, and places content_items membership.
// Image resources are stored under ResourcePathsDir at /folder/…/filename (same path JRXML uses).
func (s *Service) Create(ctx context.Context, in CreateInput) (*Resource, error) {
	if !s.Configured() {
		return nil, ErrUnavailable
	}
	rt, err := NormalizeType(in.ResourceType)
	if err != nil {
		return nil, err
	}
	display := strings.TrimSpace(in.DisplayName)
	if display == "" {
		return nil, ErrInvalidInput
	}
	if in.FolderID <= 0 {
		return nil, ErrInvalidInput
	}
	if len(in.Data) == 0 {
		return nil, ErrInvalidInput
	}

	var imgInfo *ImageInfo
	if IsImageType(rt) {
		imgInfo, err = InspectImage(in.Data, in.OriginalFilename)
		if err != nil {
			return nil, err
		}
	}

	id := uuid.New().String()
	ext := NormalizeExtension(in.Extension)
	mime := strings.TrimSpace(in.MimeType)
	if imgInfo != nil {
		ext = imgInfo.Extension
		mime = imgInfo.MimeType
	}
	if ext == "" && in.OriginalFilename != "" {
		ext = ExtensionFromFilename(in.OriginalFilename)
	}
	if ext == "" {
		ext = DefaultExtensionForType(rt)
	}
	if mime == "" {
		mime = DefaultMimeForType(rt)
	}

	origName := strings.TrimSpace(in.OriginalFilename)
	if origName == "" {
		origName = display + ext
	}
	origName = filepath.Base(origName)

	var storage string
	var sha string
	var canonical string
	if IsImageType(rt) {
		if s.Paths == nil || s.Paths.Dir == "" {
			return nil, fmt.Errorf("%w: image path store not configured", ErrUnavailable)
		}
		folderSlash := ""
		if s.Folders != nil {
			folderSlash, _ = s.Folders.SlashFolderPath(ctx, in.FolderID)
		}
		canonical = CanonicalResourcePath(folderSlash, origName)
		if canonical == "" {
			return nil, ErrInvalidInput
		}
		if err := s.Paths.Write(canonical, in.Data); err != nil {
			return nil, err
		}
		rel, ok := ResourcePathRelative(canonical)
		if !ok {
			_ = s.Paths.Remove(canonical)
			return nil, ErrInvalidInput
		}
		storage = rel
		sha = HashBytes(in.Data)
	} else {
		storage = StorageFilename(id, ext)
		sha, err = s.Files.Write(storage, in.Data)
		if err != nil {
			return nil, err
		}
	}

	var createdBy *int64
	if in.CreatedBy > 0 {
		cb := in.CreatedBy
		createdBy = &cb
	}
	res := &Resource{
		UUID:             id,
		FolderID:         in.FolderID,
		DisplayName:      display,
		ResourceType:     rt,
		OriginalFilename: origName,
		Extension:        ext,
		StorageFilename:  storage,
		MimeType:         mime,
		SHA256:           sha,
		SizeBytes:        int64(len(in.Data)),
		CreatedBy:        createdBy,
	}
	if imgInfo != nil {
		if imgInfo.Width > 0 {
			w := imgInfo.Width
			res.Width = &w
		}
		if imgInfo.Height > 0 {
			h := imgInfo.Height
			res.Height = &h
		}
		res.HasAlpha = imgInfo.HasAlpha
	}
	if err := s.Repo.Insert(ctx, res); err != nil {
		if IsImageType(rt) {
			_ = s.Paths.Remove(canonical)
		} else {
			_ = s.Files.Delete(storage)
		}
		return nil, err
	}
	if s.Folders != nil {
		if err := s.Folders.EnsureResourceInFolder(ctx, in.FolderID, id, display, in.CreatedBy); err != nil {
			_ = s.Repo.DeleteByUUID(ctx, id)
			if IsImageType(rt) {
				_ = s.Paths.Remove(canonical)
			} else {
				_ = s.Files.Delete(storage)
			}
			return nil, err
		}
	}
	if imgInfo != nil {
		_ = s.writeThumbnail(id, in.Data)
	}
	s.attachPaths(ctx, res)
	return res, nil
}

// ReplaceContent replaces file bytes for an existing resource, preserving UUID,
// display name, folder, resource type, and permissions (folder ACL).
func (s *Service) ReplaceContent(ctx context.Context, resourceUUID string, in ReplaceInput) (*Resource, error) {
	if !s.Configured() {
		return nil, ErrUnavailable
	}
	if len(in.Data) == 0 {
		return nil, ErrInvalidInput
	}
	cur, err := s.Repo.GetByUUID(ctx, resourceUUID)
	if err != nil {
		return nil, err
	}
	oldPath := s.computeResourcePath(ctx, cur)
	oldStorage := cur.StorageFilename
	wasImage := IsImageType(cur.ResourceType)

	var imgInfo *ImageInfo
	if wasImage {
		imgInfo, err = InspectImage(in.Data, firstNonEmpty(in.OriginalFilename, cur.OriginalFilename))
		if err != nil {
			return nil, err
		}
	}

	ext := NormalizeExtension(in.Extension)
	mime := strings.TrimSpace(in.MimeType)
	if imgInfo != nil {
		ext = imgInfo.Extension
		mime = imgInfo.MimeType
	}
	if ext == "" {
		ext = ExtensionFromFilename(in.OriginalFilename)
	}
	if ext == "" {
		ext = cur.Extension
	}
	if ext == "" {
		ext = DefaultExtensionForType(cur.ResourceType)
	}
	if mime == "" {
		mime = DefaultMimeForType(cur.ResourceType)
	}

	origName := strings.TrimSpace(in.OriginalFilename)
	if origName == "" {
		origName = cur.OriginalFilename
	}
	if origName == "" {
		origName = cur.DisplayName + ext
	}
	origName = filepath.Base(origName)

	cur.OriginalFilename = origName
	cur.Extension = ext
	cur.MimeType = mime
	cur.SizeBytes = int64(len(in.Data))
	cur.SHA256 = HashBytes(in.Data)
	if imgInfo != nil {
		if imgInfo.Width > 0 {
			w := imgInfo.Width
			cur.Width = &w
		} else {
			cur.Width = nil
		}
		if imgInfo.Height > 0 {
			h := imgInfo.Height
			cur.Height = &h
		} else {
			cur.Height = nil
		}
		cur.HasAlpha = imgInfo.HasAlpha
	} else {
		cur.Width = nil
		cur.Height = nil
		cur.HasAlpha = nil
	}

	if wasImage {
		if s.Paths == nil || s.Paths.Dir == "" {
			return nil, fmt.Errorf("%w: image path store not configured", ErrUnavailable)
		}
		s.attachPaths(ctx, cur)
		if err := s.Paths.Write(cur.ResourcePath, in.Data); err != nil {
			return nil, err
		}
		rel, _ := ResourcePathRelative(cur.ResourcePath)
		cur.StorageFilename = rel
		if oldPath != "" && oldPath != cur.ResourcePath {
			_ = s.Paths.Remove(oldPath)
		}
	} else {
		newStorage := StorageFilename(cur.UUID, ext)
		sha, err := s.Files.Write(newStorage, in.Data)
		if err != nil {
			return nil, err
		}
		cur.StorageFilename = newStorage
		cur.SHA256 = sha
	}

	if err := s.Repo.UpdateContent(ctx, cur); err != nil {
		return nil, err
	}
	if !wasImage && oldStorage != "" && oldStorage != cur.StorageFilename {
		_ = s.Files.Delete(oldStorage)
	}
	if wasImage {
		_ = s.writeThumbnail(cur.UUID, in.Data)
	}
	return s.Get(ctx, cur.UUID)
}

func (s *Service) writeThumbnail(uuid string, data []byte) error {
	if s.Thumbnails == nil || s.Thumbnails.Dir == "" {
		return nil
	}
	pngBytes, err := GenerateThumbnailPNG(data, defaultThumbMaxEdge)
	if err != nil {
		_ = s.Thumbnails.Delete(uuid)
		return err
	}
	return s.Thumbnails.WritePNG(uuid, pngBytes)
}

// ReadThumbnail returns a generated PNG thumbnail for an image resource.
func (s *Service) ReadThumbnail(ctx context.Context, resourceUUID string) (*Resource, []byte, error) {
	if !s.Configured() {
		return nil, nil, ErrUnavailable
	}
	res, err := s.Repo.GetByUUID(ctx, resourceUUID)
	if err != nil {
		return nil, nil, err
	}
	if !IsImageType(res.ResourceType) {
		return nil, nil, ErrInvalidInput
	}
	if s.Thumbnails == nil || s.Thumbnails.Dir == "" {
		return nil, nil, ErrNotFound
	}
	data, err := s.Thumbnails.ReadPNG(resourceUUID)
	if err != nil {
		return res, nil, err
	}
	return res, data, nil
}

// Get returns metadata for a UUID.
func (s *Service) Get(ctx context.Context, resourceUUID string) (*Resource, error) {
	if !s.Configured() {
		return nil, ErrUnavailable
	}
	res, err := s.Repo.GetByUUID(ctx, resourceUUID)
	if err != nil {
		return nil, err
	}
	s.attachPaths(ctx, res)
	return res, nil
}

// UpdateMetadata patches metadata and optionally moves folder membership.
func (s *Service) UpdateMetadata(ctx context.Context, resourceUUID string, in UpdateInput, createdBy int64) (*Resource, error) {
	if !s.Configured() {
		return nil, ErrUnavailable
	}
	cur, err := s.Repo.GetByUUID(ctx, resourceUUID)
	if err != nil {
		return nil, err
	}
	oldPath := s.computeResourcePath(ctx, cur)

	res, err := s.Repo.UpdateMetadata(ctx, resourceUUID, in)
	if err != nil {
		return nil, err
	}
	if s.Folders != nil {
		_ = s.Folders.MoveResource(ctx, res.FolderID, res.UUID, res.DisplayName, createdBy)
	}
	s.attachPaths(ctx, res)

	if IsImageType(res.ResourceType) && s.Paths != nil && oldPath != "" && res.ResourcePath != "" && oldPath != res.ResourcePath {
		data, rerr := s.Paths.Read(oldPath)
		if rerr != nil {
			data, rerr = s.Files.Read(res.StorageFilename)
		}
		if rerr == nil && len(data) > 0 {
			_ = s.Paths.Write(res.ResourcePath, data)
			_ = s.Paths.Remove(oldPath)
			if rel, ok := ResourcePathRelative(res.ResourcePath); ok {
				res.StorageFilename = rel
				_ = s.Repo.UpdateContent(ctx, res)
			}
		}
	}
	return s.Get(ctx, res.UUID)
}

// Delete removes filesystem file, DB row, thumbnail, and content_items membership.
func (s *Service) Delete(ctx context.Context, resourceUUID string) error {
	if !s.Configured() {
		return ErrUnavailable
	}
	res, err := s.Repo.GetByUUID(ctx, resourceUUID)
	if err != nil {
		return err
	}
	path := s.computeResourcePath(ctx, res)
	if err := s.Repo.DeleteByUUID(ctx, resourceUUID); err != nil {
		return err
	}
	if IsImageType(res.ResourceType) {
		_ = s.Paths.Remove(path)
	} else {
		_ = s.Files.Delete(res.StorageFilename)
	}
	if s.Thumbnails != nil {
		_ = s.Thumbnails.Delete(resourceUUID)
	}
	if s.Folders != nil {
		_ = s.Folders.RemoveResource(ctx, resourceUUID)
	}
	return nil
}

// ReadBytes loads the on-disk content for a resource UUID.
func (s *Service) ReadBytes(ctx context.Context, resourceUUID string) (*Resource, []byte, error) {
	if !s.Configured() {
		return nil, nil, ErrUnavailable
	}
	res, err := s.Repo.GetByUUID(ctx, resourceUUID)
	if err != nil {
		return nil, nil, err
	}
	s.attachPaths(ctx, res)
	if IsImageType(res.ResourceType) && s.Paths != nil && res.ResourcePath != "" {
		data, err := s.Paths.Read(res.ResourcePath)
		if err == nil {
			return res, data, nil
		}
	}
	data, err := s.Files.Read(res.StorageFilename)
	if err != nil {
		return nil, nil, err
	}
	return res, data, nil
}

// List returns resources; callers apply ACL. Folder paths are filled when Folders is set.
func (s *Service) List(ctx context.Context, f ListFilter) ([]Resource, error) {
	if !s.Configured() {
		return nil, ErrUnavailable
	}
	list, err := s.Repo.List(ctx, f)
	if err != nil {
		return nil, err
	}
	for i := range list {
		s.attachPaths(ctx, &list[i])
	}
	return list, nil
}

func (s *Service) computeResourcePath(ctx context.Context, res *Resource) string {
	if res == nil {
		return ""
	}
	folderSlash := ""
	if s.Folders != nil {
		folderSlash, _ = s.Folders.SlashFolderPath(ctx, res.FolderID)
	}
	return CanonicalResourcePath(folderSlash, res.OriginalFilename)
}

func (s *Service) attachPaths(ctx context.Context, res *Resource) {
	if res == nil || s.Folders == nil {
		return
	}
	if path, err := s.Folders.PathString(ctx, res.FolderID); err == nil {
		res.FolderPath = path
	}
	res.ResourcePath = s.computeResourcePath(ctx, res)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// FolderAdapter adapts folders.Service to FolderPlacer.
type FolderAdapter struct {
	Svc *folders.Service
}

// EnsureResourceInFolder implements FolderPlacer.
func (a FolderAdapter) EnsureResourceInFolder(ctx context.Context, folderID int64, resourceUUID, displayName string, createdBy int64) error {
	if a.Svc == nil {
		return nil
	}
	return a.Svc.EnsureResourceInFolder(ctx, folderID, resourceUUID, displayName, createdBy)
}

// MoveResource implements FolderPlacer.
func (a FolderAdapter) MoveResource(ctx context.Context, folderID int64, resourceUUID, displayName string, createdBy int64) error {
	if a.Svc == nil {
		return nil
	}
	return a.Svc.EnsureResourceInFolder(ctx, folderID, resourceUUID, displayName, createdBy)
}

// RemoveResource implements FolderPlacer.
func (a FolderAdapter) RemoveResource(ctx context.Context, resourceUUID string) error {
	if a.Svc == nil || a.Svc.Repo == nil {
		return nil
	}
	err := a.Svc.Repo.DeleteItem(ctx, folders.ContentTypeResource, resourceUUID)
	if err == folders.ErrItemNotFound {
		return nil
	}
	return err
}

// PathString implements FolderPlacer.
func (a FolderAdapter) PathString(ctx context.Context, folderID int64) (string, error) {
	if a.Svc == nil || a.Svc.Repo == nil {
		return "", nil
	}
	return a.Svc.Repo.PathString(ctx, folderID)
}

// SlashFolderPath implements FolderPlacer.
func (a FolderAdapter) SlashFolderPath(ctx context.Context, folderID int64) (string, error) {
	if a.Svc == nil {
		return "", nil
	}
	return a.Svc.SlashFolderPath(ctx, folderID)
}
