package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// CreateUsersTable creates the initial users table and related tables
func CreateUsersTable() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "000001_create_users_table",
		Migrate: func(tx *gorm.DB) error {
			// Create users table
			if err := tx.Exec(`
				CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
				
				CREATE TABLE IF NOT EXISTS users (
					id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
					username VARCHAR(255) NOT NULL UNIQUE,
					email VARCHAR(255) NOT NULL UNIQUE,
					password VARCHAR(255) NOT NULL,
					first_name VARCHAR(255) NOT NULL,
					last_name VARCHAR(255) NOT NULL,
					profile_pic_url TEXT,
					is_verified BOOLEAN DEFAULT FALSE,
					is_admin BOOLEAN DEFAULT FALSE,
					two_factor_enabled BOOLEAN DEFAULT FALSE,
					two_factor_secret VARCHAR(255),
					referral_code VARCHAR(255) NOT NULL UNIQUE,
					referred_by UUID REFERENCES users(id),
					created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
					updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
					deleted_at TIMESTAMP WITH TIME ZONE
				);
				
				CREATE INDEX idx_users_email ON users(email);
				CREATE INDEX idx_users_username ON users(username);
				CREATE INDEX idx_users_referral_code ON users(referral_code);
			`).Error; err != nil {
				return err
			}

			// Create password reset tokens table
			if err := tx.Exec(`
				CREATE TABLE IF NOT EXISTS password_reset_tokens (
					id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
					user_id UUID NOT NULL REFERENCES users(id),
					token VARCHAR(255) NOT NULL,
					expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
					created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
					UNIQUE(user_id, token)
				);
				
				CREATE INDEX idx_password_reset_tokens_token ON password_reset_tokens(token);
			`).Error; err != nil {
				return err
			}

			// Create email verification tokens table
			if err := tx.Exec(`
				CREATE TABLE IF NOT EXISTS email_verification_tokens (
					id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
					user_id UUID NOT NULL REFERENCES users(id),
					token VARCHAR(255) NOT NULL,
					expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
					created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
					UNIQUE(user_id, token)
				);
				
				CREATE INDEX idx_email_verification_tokens_token ON email_verification_tokens(token);
			`).Error; err != nil {
				return err
			}

			// Create sessions table
			if err := tx.Exec(`
				CREATE TABLE IF NOT EXISTS sessions (
					id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
					user_id UUID NOT NULL REFERENCES users(id),
					refresh_token VARCHAR(255) NOT NULL,
					user_agent TEXT,
					ip_address VARCHAR(45),
					expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
					created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
					updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
				);
				
				CREATE INDEX idx_sessions_user_id ON sessions(user_id);
				CREATE INDEX idx_sessions_refresh_token ON sessions(refresh_token);
			`).Error; err != nil {
				return err
			}

			// Create wallets table
			if err := tx.Exec(`
				CREATE TABLE IF NOT EXISTS wallets (
					id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
					user_id UUID NOT NULL UNIQUE REFERENCES users(id),
					balance DECIMAL(20, 8) DEFAULT 0,
					currency VARCHAR(3) DEFAULT 'USD',
					auto_withdraw BOOLEAN DEFAULT FALSE,
					withdraw_threshold DECIMAL(20, 8) DEFAULT 0,
					created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
					updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
				);
				
				CREATE INDEX idx_wallets_user_id ON wallets(user_id);
			`).Error; err != nil {
				return err
			}

			// Create referrals table
			return tx.Exec(`
				CREATE TABLE IF NOT EXISTS referrals (
					id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
					referrer_id UUID NOT NULL REFERENCES users(id),
					referred_id UUID NOT NULL REFERENCES users(id),
					status VARCHAR(20) DEFAULT 'pending',
					commission DECIMAL(20, 8) DEFAULT 0,
					currency VARCHAR(3) DEFAULT 'USD',
					paid_at TIMESTAMP WITH TIME ZONE,
					created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
					updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
					UNIQUE(referrer_id, referred_id)
				);
				
				CREATE INDEX idx_referrals_referrer_id ON referrals(referrer_id);
				CREATE INDEX idx_referrals_referred_id ON referrals(referred_id);
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			if err := tx.Exec("DROP TABLE IF EXISTS referrals").Error; err != nil {
				return err
			}
			if err := tx.Exec("DROP TABLE IF EXISTS wallets").Error; err != nil {
				return err
			}
			if err := tx.Exec("DROP TABLE IF EXISTS sessions").Error; err != nil {
				return err
			}
			if err := tx.Exec("DROP TABLE IF EXISTS email_verification_tokens").Error; err != nil {
				return err
			}
			if err := tx.Exec("DROP TABLE IF EXISTS password_reset_tokens").Error; err != nil {
				return err
			}
			return tx.Exec("DROP TABLE IF EXISTS users").Error
		},
	}
}
