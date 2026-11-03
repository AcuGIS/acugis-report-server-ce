//go:build unix

package fileown

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"strings"
	"syscall"
)

// Apply sets ownership (when owner/group configured) and Tomcat-friendly modes.
// If user.Lookup fails (common with static Go binaries / NSS), falls back to the
// chown(1) command so root-run services still fix ownership for Tomcat.
func Apply(path, owner, group string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	st, err := os.Stat(path)
	if err != nil {
		return err
	}
	mode := os.FileMode(0o644)
	if st.IsDir() {
		mode = 0o755
	}
	_ = os.Chmod(path, mode)

	owner = strings.TrimSpace(owner)
	group = strings.TrimSpace(group)
	if owner == "" && group == "" {
		return nil
	}

	uid, gid, lookupErr := lookup(owner, group)
	if lookupErr == nil {
		curUID, curGID := currentIDs(st)
		if uid < 0 {
			uid = curUID
		}
		if gid < 0 {
			gid = curGID
		}
		if uid >= 0 && gid >= 0 {
			if err := os.Chown(path, uid, gid); err == nil {
				return nil
			} else {
				lookupErr = err
			}
		}
	}

	// Fallback: shell out to chown (works when libc NSS resolves tomcat but
	// Go's user.Lookup does not, or os.Chown is denied for another reason).
	spec := chownSpec(owner, group)
	if spec == "" {
		if lookupErr != nil {
			return lookupErr
		}
		return nil
	}
	out, err := exec.Command("chown", spec, path).CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		if lookupErr != nil {
			return fmt.Errorf("chown %s: %v (lookup: %v)", spec, msg, lookupErr)
		}
		return fmt.Errorf("chown %s: %v", spec, msg)
	}
	return nil
}

func chownSpec(owner, group string) string {
	switch {
	case owner != "" && group != "":
		return owner + ":" + group
	case owner != "":
		return owner
	case group != "":
		return ":" + group
	default:
		return ""
	}
}

func lookup(owner, group string) (uid, gid int, err error) {
	uid, gid = -1, -1
	owner = strings.TrimSpace(owner)
	group = strings.TrimSpace(group)
	if owner == "" && group == "" {
		return -1, -1, nil
	}
	if owner != "" {
		u, e := user.Lookup(owner)
		if e != nil {
			return -1, -1, e
		}
		uid, e = strconv.Atoi(u.Uid)
		if e != nil {
			return -1, -1, e
		}
		if group == "" && u.Gid != "" {
			gid, _ = strconv.Atoi(u.Gid)
		}
	}
	if group != "" {
		g, e := user.LookupGroup(group)
		if e != nil {
			if uid >= 0 && gid >= 0 {
				return uid, gid, nil
			}
			return uid, -1, e
		}
		gid, e = strconv.Atoi(g.Gid)
		if e != nil {
			return uid, -1, e
		}
	}
	return uid, gid, nil
}

func currentIDs(st os.FileInfo) (uid, gid int) {
	uid, gid = -1, -1
	if st == nil {
		return
	}
	sys, ok := st.Sys().(*syscall.Stat_t)
	if !ok || sys == nil {
		return
	}
	return int(sys.Uid), int(sys.Gid)
}
