package utils

import (
	"fmt"
	"math/rand"
	"time"
)

// GenerateReference generates a unique reference for transactions
func GenerateReference(prefix string) string {
	// Initialize random seed
	rand.Seed(time.Now().UnixNano())
	
	// Generate random string
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, 8)
	for i := range result {
		result[i] = charset[rand.Intn(len(charset))]
	}
	
	// Format with prefix and timestamp
	timestamp := time.Now().Format("20060102")
	return fmt.Sprintf("%s_%s_%s", prefix, timestamp, string(result))
}
