package momo

import (
	"github.com/revaspay/backend/internal/config"
	"gorm.io/gorm"
)

// InitMoMoService initializes the MTN Mobile Money service with configuration
func InitMoMoService(db *gorm.DB, cfg *config.Config) *MoMoService {
	return NewMoMoService(
		db,
		cfg.MoMoSubscriptionKey,
		cfg.MoMoCollectionAPIUser,
		cfg.MoMoCollectionAPIKey,
		cfg.MoMoDisbursementAPIUser,
		cfg.MoMoDisbursementAPIKey,
		cfg.MoMoUseSandbox,
	)
}
