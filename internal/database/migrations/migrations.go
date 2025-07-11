package migrations

import (
	"log"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// migrationsList holds all migrations
var migrationsList []*gormigrate.Migration

// RunMigrations runs all database migrations
func RunMigrations(db *gorm.DB) error {
	m := gormigrate.New(db, gormigrate.DefaultOptions, migrationsList)

	if err := m.Migrate(); err != nil {
		log.Printf("Could not migrate: %v", err)
		return err
	}
	log.Printf("Migrations ran successfully")
	return nil
}
