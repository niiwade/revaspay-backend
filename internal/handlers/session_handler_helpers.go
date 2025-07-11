package handlers

import (
	"strings"
)

// extractBrowserInfo extracts browser information from user agent string
func extractBrowserInfo(userAgent string) string {
	userAgent = strings.ToLower(userAgent)
	
	if strings.Contains(userAgent, "chrome") && !strings.Contains(userAgent, "chromium") && !strings.Contains(userAgent, "edg") {
		return "Chrome"
	} else if strings.Contains(userAgent, "firefox") {
		return "Firefox"
	} else if strings.Contains(userAgent, "safari") && !strings.Contains(userAgent, "chrome") && !strings.Contains(userAgent, "android") {
		return "Safari"
	} else if strings.Contains(userAgent, "edg") {
		return "Edge"
	} else if strings.Contains(userAgent, "opera") {
		return "Opera"
	} else if strings.Contains(userAgent, "msie") || strings.Contains(userAgent, "trident") {
		return "Internet Explorer"
	}
	
	return "Other"
}

// extractOSInfo extracts operating system information from user agent string
func extractOSInfo(userAgent string) string {
	userAgent = strings.ToLower(userAgent)
	
	if strings.Contains(userAgent, "windows") {
		return "Windows"
	} else if strings.Contains(userAgent, "macintosh") || strings.Contains(userAgent, "mac os") {
		return "macOS"
	} else if strings.Contains(userAgent, "linux") && !strings.Contains(userAgent, "android") {
		return "Linux"
	} else if strings.Contains(userAgent, "iphone") || strings.Contains(userAgent, "ipad") || strings.Contains(userAgent, "ipod") {
		return "iOS"
	} else if strings.Contains(userAgent, "android") {
		return "Android"
	}
	
	return "Other"
}

// extractDeviceType determines the device type from user agent string
func extractDeviceType(userAgent string) string {
	userAgent = strings.ToLower(userAgent)
	
	if strings.Contains(userAgent, "mobile") || 
	   (strings.Contains(userAgent, "android") && !strings.Contains(userAgent, "tablet")) || 
	   strings.Contains(userAgent, "iphone") || 
	   strings.Contains(userAgent, "ipod") {
		return "Mobile"
	} else if strings.Contains(userAgent, "tablet") || strings.Contains(userAgent, "ipad") {
		return "Tablet"
	}
	
	return "Desktop"
}
