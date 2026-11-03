package main

import (
	"log/slog"
	"os"
	"strings"

	"github.com/acugis/acugis-report-server-go/internal/config"
	"github.com/acugis/acugis-report-server-go/internal/edition"
	"github.com/acugis/acugis-report-server-go/internal/license"
)

func openLicense(cfg config.Config, logger *slog.Logger) (*license.Manager, *edition.Service) {
	ed := edition.New(edition.EditionCommunity)
	pub, err := license.ProductionPublicKey()
	if err != nil {
		logger.Error("license: verification key unavailable; effective edition=community")
		return nil, ed
	}
	mgr, err := license.NewManager(license.Options{
		StateDir:       cfg.LicenseStateDir,
		ServerURL:      cfg.LicenseServerURL,
		ProductVersion: cfg.AppVersion,
		PublicKey:      pub,
		Logger:         logger,
	})
	if err != nil {
		logger.Error("license: verifier unavailable; effective edition=community")
		return nil, ed
	}
	if err := mgr.Load(); err != nil {
		logger.Error("license: installation identity is invalid; effective edition=community")
	}
	mgr.LogStartup()
	ed.BindLive(func() edition.Edition {
		if mgr.ProfessionalEntitled() {
			return edition.EditionProfessional
		}
		return edition.EditionCommunity
	})
	return mgr, ed
}

func openDesignerLicense(cfg config.Config, logger *slog.Logger) *license.Manager {
	pub, err := license.ProductionPublicKey()
	if err != nil {
		logger.Error("license: Report Designer Web verification key unavailable")
		return nil
	}
	version := strings.TrimSpace(os.Getenv("ACUGIS_DESIGNER_WEB_VERSION"))
	if version == "" {
		version = "1.0.0"
	}
	mgr, err := license.NewManager(license.Options{
		StateDir:                    cfg.LicenseStateDir,
		ServerURL:                   cfg.LicenseServerURL,
		ProductVersion:              version,
		PublicKey:                   pub,
		Logger:                      logger,
		ProductCode:                 license.ProductReportDesignerWeb,
		LicenseFile:                 license.DesignerLicenseFileName,
		RequireExistingInstallation: true,
	})
	if err != nil {
		logger.Error("license: Report Designer Web verifier unavailable")
		return nil
	}
	if err := mgr.Load(); err != nil {
		logger.Error("license: Report Designer Web installation identity is invalid")
	}
	mgr.LogStartup()
	return mgr
}

func versionEdition() *edition.Service {
	cfg, err := config.Load()
	if err != nil {
		return edition.New(edition.EditionCommunity)
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	_, ed := openLicense(cfg, logger)
	return ed
}
