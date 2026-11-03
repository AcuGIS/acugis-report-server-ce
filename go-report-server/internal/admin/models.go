package admin

import "time"

// GroupRef is a minimal group row for checkbox lists.
type GroupRef struct {
	ID   int64
	Name string
}

// UserRef is a minimal user row for membership checkboxes.
type UserRef struct {
	ID       int64
	Username string
}

// UserRow is a dashboard user with aggregated group names (users.php table).
type UserRow struct {
	ID         int64
	Username   string
	Email      string
	IsAdmin    bool
	Enabled    bool
	CreatedAt  time.Time
	GroupNames string
	// HasLocalPassword is true when password_hash is a usable local login credential.
	HasLocalPassword bool
}

// GroupRow is an auth group with aggregated member usernames (groups.php table).
type GroupRow struct {
	ID          int64
	Name        string
	Description string
	Members     string
}

// SaveUserInput mirrors admin/users.php POST save fields.
type SaveUserInput struct {
	ID       int64
	Username string
	Email    string
	Password string // empty on update = leave unchanged
	IsAdmin  bool
	Enabled  bool
}

// SaveGroupInput mirrors admin/groups.php POST save fields.
type SaveGroupInput struct {
	ID          int64
	Name        string
	Description string
}
