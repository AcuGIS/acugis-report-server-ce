// Command jrs-auth-check validates a PHP session token against the application
// database without printing the raw token or its hash.
//
// Safe usage:
//
//	# Preferred: pipe the raw cookie value on stdin (local testing only)
//	printf '%s' "$TOKEN" | go run ./cmd/jrs-auth-check
//
//	# Or set a protected env var (do not export permanently)
//	JRS_AUTH_CHECK_TOKEN="$TOKEN" go run ./cmd/jrs-auth-check
//
// Do not pass the token as a CLI argument (shell history / process listings).
package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/acugis/acugis-report-server-go/internal/auth"
	"github.com/acugis/acugis-report-server-go/internal/config"
	"github.com/acugis/acugis-report-server-go/internal/database"
)

func main() {
	os.Exit(run())
}

func run() int {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		return 2
	}

	raw, err := readToken()
	if err != nil {
		fmt.Fprintf(os.Stderr, "token input error: %v\n", err)
		return 2
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db, err := database.Open(ctx, cfg.Database, logger)
	if err != nil {
		fmt.Println("status: database_error")
		return 1
	}
	defer db.Close()

	repo := auth.NewRepository(db.PGX())
	svc := auth.NewService(repo, auth.Options{
		CookieName:   cfg.SessionCookieName,
		AuthDisabled: cfg.AuthDisabled,
		Logger:       logger,
	})

	res := svc.CheckToken(ctx, raw)
	switch res.Status {
	case auth.StatusAuthenticated:
		fmt.Println("status: valid")
		if res.User != nil {
			fmt.Printf("user_id: %d\n", res.User.ID)
			fmt.Printf("username: %s\n", res.User.Username)
		}
		return 0
	case auth.StatusAuthDisabled:
		fmt.Println("status: auth_disabled")
		fmt.Println("username: local-dev")
		return 0
	case auth.StatusExpired:
		fmt.Println("status: expired")
		return 1
	case auth.StatusDisabledUser:
		fmt.Println("status: disabled_user")
		return 1
	case auth.StatusDatabaseError:
		fmt.Println("status: database_error")
		return 1
	case auth.StatusAnonymous:
		fmt.Println("status: invalid")
		return 1
	default:
		fmt.Println("status: invalid")
		return 1
	}
}

func readToken() (string, error) {
	if v := strings.TrimSpace(os.Getenv("JRS_AUTH_CHECK_TOKEN")); v != "" {
		return v, nil
	}
	fi, err := os.Stdin.Stat()
	if err != nil {
		return "", err
	}
	if (fi.Mode() & os.ModeCharDevice) != 0 {
		return "", fmt.Errorf("provide token via stdin or JRS_AUTH_CHECK_TOKEN (not as a CLI argument)")
	}
	data, err := io.ReadAll(bufio.NewReader(os.Stdin))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}
