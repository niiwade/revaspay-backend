package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func createJobsTableMigration() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "000002_create_jobs_table",
		Migrate: func(tx *gorm.DB) error {
			// Create jobs table for the queue system
			return tx.Exec(`
				CREATE TABLE IF NOT EXISTS jobs (
					id SERIAL PRIMARY KEY,
					job_type VARCHAR(100) NOT NULL,
					payload JSONB,
					status VARCHAR(50) NOT NULL DEFAULT 'pending',
					attempts INT NOT NULL DEFAULT 0,
					max_attempts INT NOT NULL DEFAULT 3,
					created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
					updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
					scheduled_at TIMESTAMP WITH TIME ZONE,
					retry_at TIMESTAMP WITH TIME ZONE,
					started_at TIMESTAMP WITH TIME ZONE,
					completed_at TIMESTAMP WITH TIME ZONE,
					failed_at TIMESTAMP WITH TIME ZONE,
					error TEXT,
					result JSONB,
					priority INT NOT NULL DEFAULT 0,
					worker_id VARCHAR(100),
					lock_expires_at TIMESTAMP WITH TIME ZONE
				)
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec("DROP TABLE IF EXISTS jobs").Error
		},
	}
}

func init() {
	migrationsList = append(migrationsList, createJobsTableMigration())
}
