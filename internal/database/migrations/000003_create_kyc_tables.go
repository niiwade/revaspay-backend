package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func createKYCTablesMigration() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "000003_create_kyc_tables",
		Migrate: func(tx *gorm.DB) error {
			// Create KYC verification table
			if err := tx.Exec(`
				CREATE TABLE IF NOT EXISTS kyc_verifications (
					id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
					user_id UUID NOT NULL REFERENCES users(id),
					status VARCHAR(20) NOT NULL DEFAULT 'pending',
					session_id VARCHAR(255),
					workflow_id VARCHAR(255),
					verification_url TEXT,
					id_doc_type VARCHAR(50),
					id_doc_number VARCHAR(100),
					id_doc_country VARCHAR(2),
					id_doc_expiry TIMESTAMP WITH TIME ZONE,
					full_name VARCHAR(255),
					date_of_birth TIMESTAMP WITH TIME ZONE,
					address TEXT,
					report_url TEXT,
					admin_notes TEXT,
					rejection_reason TEXT,
					created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
					updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
					deleted_at TIMESTAMP WITH TIME ZONE
				);
				
				CREATE INDEX idx_kyc_verifications_user_id ON kyc_verifications(user_id);
				CREATE INDEX idx_kyc_verifications_status ON kyc_verifications(status);
			`).Error; err != nil {
				return err
			}

			// Create KYC documents table
			if err := tx.Exec(`
				CREATE TABLE IF NOT EXISTS kyc_documents (
					id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
					verification_id UUID NOT NULL REFERENCES kyc_verifications(id),
					type VARCHAR(50) NOT NULL,
					file_path TEXT NOT NULL,
					file_name VARCHAR(255) NOT NULL,
					file_size BIGINT,
					mime_type VARCHAR(100),
					uploaded_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
					didit_doc_id VARCHAR(255)
				);
				
				CREATE INDEX idx_kyc_documents_verification_id ON kyc_documents(verification_id);
				CREATE INDEX idx_kyc_documents_type ON kyc_documents(type);
			`).Error; err != nil {
				return err
			}

			// Create KYC verification history table
			return tx.Exec(`
				CREATE TABLE IF NOT EXISTS kyc_verification_histories (
					id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
					verification_id UUID NOT NULL REFERENCES kyc_verifications(id),
					previous_status VARCHAR(20) NOT NULL,
					new_status VARCHAR(20) NOT NULL,
					changed_by UUID REFERENCES users(id),
					notes TEXT,
					created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
				);
				
				CREATE INDEX idx_kyc_verification_histories_verification_id ON kyc_verification_histories(verification_id);
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			if err := tx.Exec("DROP TABLE IF EXISTS kyc_verification_histories").Error; err != nil {
				return err
			}
			if err := tx.Exec("DROP TABLE IF EXISTS kyc_documents").Error; err != nil {
				return err
			}
			return tx.Exec("DROP TABLE IF EXISTS kyc_verifications").Error
		},
	}
}

func init() {
	migrationsList = append(migrationsList, createKYCTablesMigration())
}
