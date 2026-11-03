package acl

import "context"

// InheritanceLookup resolves folder ACL inheritance for content and folders.
// Implemented by the folders package; optional on DBService.
type InheritanceLookup interface {
	// ContentFolderChain returns nearest-first folder IDs for a content item.
	// inherit is false when the item opted out of folder ACL inheritance.
	ContentFolderChain(ctx context.Context, contentType, contentKey string) (folderIDs []int64, inherit bool, err error)
	// FolderAncestorIDs returns nearest-first ancestor folder IDs excluding self.
	// Walk stops before a folder with acl_inherit=false (that folder is not included).
	FolderAncestorIDs(ctx context.Context, folderID int64) (ancestorIDs []int64, err error)
}

// SourceInherited is defined in errors.go (DecisionSource constants).

func (s *DBService) SetInheritance(lookup InheritanceLookup) {
	if s == nil {
		return
	}
	s.inheritance = lookup
}
