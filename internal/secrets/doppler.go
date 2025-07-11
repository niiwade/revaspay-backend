package secrets

// Package secrets provides secure key management using Doppler

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// DopplerClient provides access to secrets stored in Doppler
type DopplerClient struct {
	Project     string
	Config      string
	initialized bool
}

// NewDopplerClient creates a new Doppler client
func NewDopplerClient(project, config string) *DopplerClient {
	return &DopplerClient{
		Project:     project,
		Config:      config,
		initialized: false,
	}
}

// Initialize checks if Doppler CLI is installed and configured
func (d *DopplerClient) Initialize() error {
	// Check if Doppler CLI is installed
	_, err := exec.LookPath("doppler")
	if err != nil {
		return fmt.Errorf("doppler CLI not found: %w", err)
	}

	// Set initialized flag
	d.initialized = true
	return nil
}

// GetSecret retrieves a secret from Doppler
func (d *DopplerClient) GetSecret(key string) (string, error) {
	if !d.initialized {
		if err := d.Initialize(); err != nil {
			return "", err
		}
	}

	// First try to get from environment (for local development with doppler run)
	value := os.Getenv(key)
	if value != "" {
		return value, nil
	}

	// If not in environment, try to get directly from Doppler CLI
	cmd := exec.Command("doppler", "secrets", "get", key, 
		"--project", d.Project, 
		"--config", d.Config, 
		"--plain")
	
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get secret %s: %w", key, err)
	}

	return strings.TrimSpace(string(output)), nil
}

// GetSecretWithFallback gets a secret from Doppler with a fallback value
func (d *DopplerClient) GetSecretWithFallback(key, fallback string) string {
	value, err := d.GetSecret(key)
	if err != nil || value == "" {
		return fallback
	}
	return value
}
