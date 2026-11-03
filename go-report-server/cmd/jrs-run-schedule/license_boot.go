package main

import (
	"log/slog"

	"github.com/acugis/acugis-report-server-go/internal/config"
	"github.com/acugis/acugis-report-server-go/internal/edition"
	"github.com/acugis/acugis-report-server-go/internal/license"
)

// scheduleEdition resolves the same ACL1 entitlement as jrs-server.
// A one-shot runner does not block on a licensing-server refresh.
func scheduleEdition(cfg config.Config, logger *slog.Logger) *edition.Service {
	ed := edition.New(edition.EditionCommunity)
	pub, err := license.ProductionPublicKey()
	if err != nil {
		logger.Error("license: verification key unavailable; effective edition=community")
		return ed
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
		return ed
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
	return ed
}
