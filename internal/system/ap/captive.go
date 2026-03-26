package ap

import (
	"net/http"
)

// CaptivePortalPaths returns the OS-specific captive portal detection paths.
func CaptivePortalPaths() []string {
	return []string{
		"/hotspot-detect.html",       // Apple
		"/library/test/success.html", // Apple
		"/generate_204",              // Android
		"/gen_204",                   // Android
		"/connecttest.txt",           // Windows
		"/ncsi.txt",                  // Windows
		"/redirect",                  // Windows
		"/success.txt",               // Firefox
		"/canonical.html",            // Ubuntu
	}
}

// IsCaptivePortalRequest checks if the request is a captive portal detection probe.
func IsCaptivePortalRequest(r *http.Request) bool {
	for _, path := range CaptivePortalPaths() {
		if r.URL.Path == path {
			return true
		}
	}
	return false
}
