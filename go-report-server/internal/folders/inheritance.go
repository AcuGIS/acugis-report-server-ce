package folders

import (
	"context"
	"errors"
	"strconv"
)

// ContentFolderChain implements acl.InheritanceLookup for content items.
func (r *Repository) ContentFolderChain(ctx context.Context, contentType, contentKey string) ([]int64, bool, error) {
	if !r.Configured() {
		return nil, false, nil
	}
	it, err := r.GetItemByKey(ctx, contentType, contentKey)
	if err != nil {
		if errors.Is(err, ErrItemNotFound) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if !it.ACLInherit {
		return nil, false, nil
	}
	ids, err := r.FolderIDsInChain(ctx, it.FolderID)
	if err != nil {
		return nil, false, err
	}
	// Stop walking past a folder that opted out — include that folder's own ACL
	// but not its parents (FolderIDsInChain is nearest-first including self).
	var out []int64
	for _, id := range ids {
		out = append(out, id)
		inherit, _, gerr := r.GetFolderMeta(ctx, id)
		if gerr != nil {
			return nil, false, gerr
		}
		if !inherit {
			break
		}
	}
	return out, true, nil
}

// FolderAncestorIDs implements acl.InheritanceLookup for folders.
func (r *Repository) FolderAncestorIDs(ctx context.Context, folderID int64) ([]int64, error) {
	if !r.Configured() || folderID <= 0 {
		return nil, nil
	}
	f, err := r.Get(ctx, folderID)
	if err != nil {
		return nil, err
	}
	if !f.ACLInherit || f.ParentID == nil {
		return nil, nil
	}
	ids, err := r.FolderIDsInChain(ctx, *f.ParentID)
	if err != nil {
		return nil, err
	}
	var out []int64
	for _, id := range ids {
		out = append(out, id)
		inherit, _, gerr := r.GetFolderMeta(ctx, id)
		if gerr != nil {
			return nil, gerr
		}
		if !inherit {
			break
		}
	}
	return out, nil
}

// ParseFolderID parses a path/query folder id.
func ParseFolderID(s string) (int64, error) {
	return strconv.ParseInt(s, 10, 64)
}
