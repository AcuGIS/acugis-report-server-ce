package folders

import (
	"context"
	"fmt"
	"strings"
)

// Service orchestrates folder operations and browse/search helpers.
type Service struct {
	Repo *Repository
}

// NewService constructs a folders service.
func NewService(repo *Repository) *Service {
	return &Service{Repo: repo}
}

// Configured reports whether the repository can run queries.
func (s *Service) Configured() bool {
	return s != nil && s.Repo != nil && s.Repo.Configured()
}

// EnsureHome ensures the root folder exists (call on startup).
func (s *Service) EnsureHome(ctx context.Context) (*Folder, error) {
	if !s.Configured() {
		return nil, ErrUnavailable
	}
	return s.Repo.EnsureHome(ctx)
}

// Browse returns folder children (folders first) plus breadcrumbs.
func (s *Service) Browse(ctx context.Context, folderID int64) (*BrowseResult, error) {
	if !s.Configured() {
		return nil, ErrUnavailable
	}
	var folder *Folder
	var err error
	if folderID <= 0 {
		folder, err = s.Repo.GetRoot(ctx)
	} else {
		folder, err = s.Repo.Get(ctx, folderID)
	}
	if err != nil {
		return nil, err
	}
	chain, err := s.Repo.AncestorChain(ctx, folder.ID)
	if err != nil {
		return nil, err
	}
	crumbs := make([]Breadcrumb, 0, len(chain))
	for _, f := range chain {
		crumbs = append(crumbs, Breadcrumb{ID: f.ID, Name: f.Name})
	}
	children, err := s.Repo.ListChildren(ctx, folder.ID)
	if err != nil {
		return nil, err
	}
	items, err := s.Repo.ListItems(ctx, folder.ID)
	if err != nil {
		return nil, err
	}
	path, _ := s.Repo.PathString(ctx, folder.ID)
	folder.Path = path
	for i := range items {
		items[i].FolderPath = path
	}
	return &BrowseResult{
		Folder:      folder,
		Breadcrumbs: crumbs,
		Folders:     children,
		Items:       items,
	}, nil
}

// Tree builds the full navigation tree from Home.
func (s *Service) Tree(ctx context.Context) (*TreeNode, error) {
	if !s.Configured() {
		return nil, ErrUnavailable
	}
	all, err := s.Repo.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	byParent := map[int64][]Folder{}
	var root *Folder
	for i := range all {
		f := all[i]
		if f.ParentID == nil {
			cp := f
			root = &cp
			continue
		}
		byParent[*f.ParentID] = append(byParent[*f.ParentID], f)
	}
	if root == nil {
		home, err := s.Repo.EnsureHome(ctx)
		if err != nil {
			return nil, err
		}
		root = home
	}
	var build func(f Folder) TreeNode
	build = func(f Folder) TreeNode {
		n := TreeNode{
			ID:       f.ID,
			ParentID: f.ParentID,
			Name:     f.Name,
			Icon:     f.Icon,
		}
		for _, c := range byParent[f.ID] {
			n.Children = append(n.Children, build(c))
		}
		return n
	}
	node := build(*root)
	return &node, nil
}

// Create creates a folder under parent.
func (s *Service) Create(ctx context.Context, in CreateFolderInput) (*Folder, error) {
	if !s.Configured() {
		return nil, ErrUnavailable
	}
	if in.ParentID <= 0 {
		home, err := s.Repo.GetRoot(ctx)
		if err != nil {
			return nil, err
		}
		in.ParentID = home.ID
	}
	if _, err := s.Repo.Get(ctx, in.ParentID); err != nil {
		return nil, err
	}
	return s.Repo.Create(ctx, in)
}

// Update renames/patches a folder.
func (s *Service) Update(ctx context.Context, id int64, in UpdateFolderInput) (*Folder, error) {
	if !s.Configured() {
		return nil, ErrUnavailable
	}
	return s.Repo.Update(ctx, id, in)
}

// MoveFolder reparents with cycle detection.
func (s *Service) MoveFolder(ctx context.Context, id int64, in MoveFolderInput) (*Folder, error) {
	if !s.Configured() {
		return nil, ErrUnavailable
	}
	return s.Repo.Move(ctx, id, in)
}

// DeleteFolder removes a folder subject to empty/recursive policy.
func (s *Service) DeleteFolder(ctx context.Context, id int64, opts DeleteOptions) error {
	if !s.Configured() {
		return ErrUnavailable
	}
	return s.Repo.Delete(ctx, id, opts)
}

// MoveContent places a report/dashboard/etc into a folder.
func (s *Service) MoveContent(ctx context.Context, in MoveItemInput) (*Item, error) {
	if !s.Configured() {
		return nil, ErrUnavailable
	}
	if in.FolderID <= 0 {
		home, err := s.Repo.GetRoot(ctx)
		if err != nil {
			return nil, err
		}
		in.FolderID = home.ID
	}
	return s.Repo.UpsertItem(ctx, in)
}

// DuplicateFolder creates an empty sibling copy named "Copy of …".
func (s *Service) DuplicateFolder(ctx context.Context, id int64, createdBy int64) (*Folder, error) {
	if !s.Configured() {
		return nil, ErrUnavailable
	}
	src, err := s.Repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if src.ParentID == nil {
		return nil, ErrRootImmutable
	}
	name := "Copy of " + src.Name
	inherit := src.ACLInherit
	return s.Repo.Create(ctx, CreateFolderInput{
		ParentID:    *src.ParentID,
		Name:        name,
		Description: src.Description,
		Icon:        src.Icon,
		SortOrder:   src.SortOrder,
		ACLInherit:  &inherit,
		CreatedBy:   createdBy,
	})
}

// Search runs cross-folder search with path display.
func (s *Service) Search(ctx context.Context, query string, limit int) ([]SearchHit, error) {
	if !s.Configured() {
		return nil, ErrUnavailable
	}
	return s.Repo.SearchFoldersAndItems(ctx, query, limit)
}

// Get returns a folder by id.
func (s *Service) Get(ctx context.Context, id int64) (*Folder, error) {
	if !s.Configured() {
		return nil, ErrUnavailable
	}
	f, err := s.Repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	f.Path, _ = s.Repo.PathString(ctx, f.ID)
	return f, nil
}

// EnsureReportInFolder places a report in folderID, or in Home when folderID is
// not positive. It does not write {key}.resource-base. Designer reports keep
// an isolated resource base that must not be replaced with the shared folder path.
func (s *Service) EnsureReportInFolder(ctx context.Context, folderID int64, reportKey, displayName string, createdBy int64) error {
	if !s.Configured() {
		return ErrUnavailable
	}
	reportKey = strings.TrimSpace(reportKey)
	if reportKey == "" {
		return ErrBadRequest
	}
	if folderID <= 0 {
		return s.EnsureReportInHome(ctx, reportKey, displayName, createdBy)
	}
	if _, err := s.Repo.Get(ctx, folderID); err != nil {
		return err
	}
	display := strings.TrimSpace(displayName)
	if display == "" {
		display = reportKey
	}
	_, err := s.Repo.UpsertItem(ctx, MoveItemInput{
		FolderID:    folderID,
		ContentType: ContentTypeReport,
		ContentKey:  reportKey,
		DisplayName: display,
		CreatedBy:   createdBy,
	})
	return err
}

// EnsureReportInHome places a report in Home if it has no folder membership yet.
func (s *Service) EnsureReportInHome(ctx context.Context, reportKey, displayName string, createdBy int64) error {
	if !s.Configured() {
		return nil
	}
	_, err := s.Repo.GetItemByKey(ctx, ContentTypeReport, reportKey)
	if err == nil {
		return nil
	}
	if err != ErrItemNotFound {
		return err
	}
	home, err := s.Repo.EnsureHome(ctx)
	if err != nil {
		return err
	}
	_, err = s.Repo.UpsertItem(ctx, MoveItemInput{
		FolderID:    home.ID,
		ContentType: ContentTypeReport,
		ContentKey:  reportKey,
		DisplayName: displayName,
		CreatedBy:   createdBy,
	})
	return err
}

// RemoveReport removes a report's folder membership (and favorites). Missing items are ignored.
func (s *Service) RemoveReport(ctx context.Context, reportKey string) error {
	if !s.Configured() {
		return nil
	}
	reportKey = strings.TrimSpace(reportKey)
	if reportKey == "" {
		return nil
	}
	_ = s.Repo.ClearFavoritesForContent(ctx, ContentTypeReport, reportKey)
	err := s.Repo.DeleteItem(ctx, ContentTypeReport, reportKey)
	if err == ErrItemNotFound {
		return nil
	}
	return err
}

// UpdateReportDisplayName refreshes the catalog title for an existing folder membership.
// Reports with no membership are left alone (they surface as unfiled under Home).
func (s *Service) UpdateReportDisplayName(ctx context.Context, reportKey, displayName string) error {
	if !s.Configured() {
		return nil
	}
	reportKey = strings.TrimSpace(reportKey)
	if reportKey == "" {
		return nil
	}
	it, err := s.Repo.GetItemByKey(ctx, ContentTypeReport, reportKey)
	if err != nil {
		if err == ErrItemNotFound {
			return nil
		}
		return err
	}
	display := strings.TrimSpace(displayName)
	if display == "" {
		display = reportKey
	}
	if it.DisplayName == display {
		return nil
	}
	sortOrder := it.SortOrder
	inherit := it.ACLInherit
	_, err = s.Repo.UpsertItem(ctx, MoveItemInput{
		FolderID:    it.FolderID,
		ContentType: ContentTypeReport,
		ContentKey:  reportKey,
		DisplayName: display,
		SortOrder:   &sortOrder,
		ACLInherit:  &inherit,
	})
	return err
}

// EnsureResourceInFolder places or moves a content resource into the given folder.
// If folderID <= 0, the resource is placed in Home.
func (s *Service) EnsureResourceInFolder(ctx context.Context, folderID int64, resourceUUID, displayName string, createdBy int64) error {
	if !s.Configured() {
		return nil
	}
	resourceUUID = strings.TrimSpace(resourceUUID)
	if resourceUUID == "" {
		return ErrBadRequest
	}
	if folderID <= 0 {
		home, err := s.Repo.EnsureHome(ctx)
		if err != nil {
			return err
		}
		folderID = home.ID
	}
	_, err := s.Repo.UpsertItem(ctx, MoveItemInput{
		FolderID:    folderID,
		ContentType: ContentTypeResource,
		ContentKey:  resourceUUID,
		DisplayName: displayName,
		CreatedBy:   createdBy,
	})
	return err
}

// EnsureExplorerViewInFolder places or moves a saved Data Explorer view into a folder.
// contentKey must be the stable "owner/viewKey" ref. folderID <= 0 places in Home.
func (s *Service) EnsureExplorerViewInFolder(ctx context.Context, folderID int64, contentKey, displayName string, createdBy int64) error {
	_, err := s.EnsureExplorerViewInFolderItem(ctx, folderID, contentKey, displayName, createdBy)
	return err
}

// EnsureExplorerViewInFolderItem is EnsureExplorerViewInFolder but returns the content_items row.
func (s *Service) EnsureExplorerViewInFolderItem(ctx context.Context, folderID int64, contentKey, displayName string, createdBy int64) (*Item, error) {
	if !s.Configured() {
		return nil, nil
	}
	contentKey = strings.TrimSpace(contentKey)
	if contentKey == "" {
		return nil, ErrBadRequest
	}
	if folderID <= 0 {
		home, err := s.Repo.EnsureHome(ctx)
		if err != nil {
			return nil, err
		}
		folderID = home.ID
	}
	display := strings.TrimSpace(displayName)
	if display == "" {
		display = contentKey
	}
	return s.Repo.UpsertItem(ctx, MoveItemInput{
		FolderID:    folderID,
		ContentType: ContentTypeExplorerView,
		ContentKey:  contentKey,
		DisplayName: display,
		CreatedBy:   createdBy,
	})
}

// RegisterExplorerViewIfMissing ensures a content_items row exists for an explorer view
// without moving an already-filed view. Unfiled legacy views are placed in Home.
// This gives stable numeric IDs for CLI/API lookup and permissions without inventing IDs.
func (s *Service) RegisterExplorerViewIfMissing(ctx context.Context, contentKey, displayName string, createdBy int64) (*Item, error) {
	if !s.Configured() {
		return nil, nil
	}
	contentKey = strings.TrimSpace(contentKey)
	if contentKey == "" {
		return nil, ErrBadRequest
	}
	it, err := s.Repo.GetItemByKey(ctx, ContentTypeExplorerView, contentKey)
	if err == nil {
		return it, nil
	}
	if err != ErrItemNotFound {
		return nil, err
	}
	return s.EnsureExplorerViewInFolderItem(ctx, 0, contentKey, displayName, createdBy)
}

// GetItemByID returns a content_items row by id.
func (s *Service) GetItemByID(ctx context.Context, id int64) (*Item, error) {
	if !s.Configured() {
		return nil, ErrUnavailable
	}
	return s.Repo.GetItemByID(ctx, id)
}

// RemoveExplorerView removes folder membership (and favorites) for a saved view ref.
func (s *Service) RemoveExplorerView(ctx context.Context, contentKey string) error {
	if !s.Configured() {
		return nil
	}
	contentKey = strings.TrimSpace(contentKey)
	if contentKey == "" {
		return nil
	}
	_ = s.Repo.ClearFavoritesForContent(ctx, ContentTypeExplorerView, contentKey)
	err := s.Repo.DeleteItem(ctx, ContentTypeExplorerView, contentKey)
	if err == ErrItemNotFound {
		return nil
	}
	return err
}

// UpdateExplorerViewDisplayName refreshes the catalog title when a view is renamed.
func (s *Service) UpdateExplorerViewDisplayName(ctx context.Context, contentKey, displayName string) error {
	if !s.Configured() {
		return nil
	}
	contentKey = strings.TrimSpace(contentKey)
	if contentKey == "" {
		return nil
	}
	it, err := s.Repo.GetItemByKey(ctx, ContentTypeExplorerView, contentKey)
	if err != nil {
		if err == ErrItemNotFound {
			return nil
		}
		return err
	}
	display := strings.TrimSpace(displayName)
	if display == "" {
		display = contentKey
	}
	if it.DisplayName == display {
		return nil
	}
	sortOrder := it.SortOrder
	inherit := it.ACLInherit
	_, err = s.Repo.UpsertItem(ctx, MoveItemInput{
		FolderID:    it.FolderID,
		ContentType: ContentTypeExplorerView,
		ContentKey:  contentKey,
		DisplayName: display,
		SortOrder:   &sortOrder,
		ACLInherit:  &inherit,
	})
	return err
}

// SlashFolderPath returns a JRXML-style folder path without a trailing filename.
// Home (root) returns ""; folder finance/images returns "/finance/images".
func (s *Service) SlashFolderPath(ctx context.Context, folderID int64) (string, error) {
	if s == nil || s.Repo == nil {
		return "", ErrUnavailable
	}
	chain, err := s.Repo.AncestorChain(ctx, folderID)
	if err != nil {
		return "", err
	}
	parts := make([]string, 0, len(chain))
	for _, f := range chain {
		if f.ParentID == nil {
			continue // skip Home
		}
		name := strings.TrimSpace(f.Name)
		if name == "" || strings.ContainsAny(name, `/\`) {
			continue
		}
		parts = append(parts, name)
	}
	if len(parts) == 0 {
		return "", nil
	}
	return "/" + strings.Join(parts, "/"), nil
}

// FormatPath joins breadcrumb names.
func FormatPath(crumbs []Breadcrumb) string {
	parts := make([]string, 0, len(crumbs))
	for _, c := range crumbs {
		parts = append(parts, c.Name)
	}
	return strings.Join(parts, " > ")
}

// FolderResourceKey returns the ACL resource_key for a folder id.
func FolderResourceKey(id int64) string {
	return fmt.Sprintf("%d", id)
}
