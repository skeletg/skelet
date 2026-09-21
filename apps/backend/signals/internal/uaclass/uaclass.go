// Package uaclass says whether a hit looks like a machine, and which device, client and OS it came from A standard-library leaf, since both sides of the Signals boundary classify hits - core's email pixel and the suite's web beacon
package uaclass

import (
	"strings"
	"time"
)

// UA substrings for a non-human open: image proxies, mail-client prefetchers, security scanners - Apple Mail Privacy Protection has no distinctive UA and is caught by the open-too-soon timing rule instead, and this stays a list so it can move to config later
var machineMarkers = []string{
	"googleimageproxy", "yahoomailproxy", "yahoo! slurp",
	"bingpreview", "skypeuripreview", "microsoft office", "ms-office",
	"proofpoint", "mimecast", "barracuda", "messagelabs", "symantec", "cloudmark",
	"bot", "crawler", "spider", "scanner", "preview", "monitor",
	"curl", "wget", "python-requests", "go-http-client", "headlesschrome",
}

type Enrichment struct {
	Machine bool   // likely proxy/scanner/prefetch - flagged, never dropped
	Device  string // mobile | tablet | desktop | unknown
	Client  string // best-effort mail-client / browser family
	OS      string // best-effort operating-system family
}

// Flags likely machine opens so human counts stay honest - sinceSend is how long after the send the hit arrived, and a hit within a few seconds is almost always a prefetch or scanner
func Classify(ua string, sinceSend time.Duration) Enrichment {
	low := strings.ToLower(strings.TrimSpace(ua))
	machine := sinceSend >= 0 && sinceSend < 3*time.Second
	if !machine {
		for _, m := range machineMarkers {
			if strings.Contains(low, m) {
				machine = true
				break
			}
		}
	}
	return Enrichment{Machine: machine, Device: deviceOf(low), Client: clientOf(low), OS: osOf(low)}
}

// Order matters - iOS and Android are checked before the desktop families, since mobile UAs also contain "like Mac"
func osOf(low string) string {
	switch {
	case low == "":
		return "unknown"
	case strings.Contains(low, "iphone") || strings.Contains(low, "ipad") || strings.Contains(low, "ipod"):
		return "iOS"
	case strings.Contains(low, "android"):
		return "Android"
	case strings.Contains(low, "windows"):
		return "Windows"
	case strings.Contains(low, "mac os") || strings.Contains(low, "macintosh"):
		return "macOS"
	case strings.Contains(low, "cros"):
		return "ChromeOS"
	case strings.Contains(low, "linux"):
		return "Linux"
	default:
		return "other"
	}
}

func deviceOf(low string) string {
	switch {
	case low == "":
		return "unknown"
	case strings.Contains(low, "ipad") || strings.Contains(low, "tablet"):
		return "tablet"
	case strings.Contains(low, "iphone") || strings.Contains(low, "android") || strings.Contains(low, "mobile"):
		return "mobile"
	default:
		return "desktop"
	}
}

func clientOf(low string) string {
	switch {
	case low == "":
		return "unknown"
	case strings.Contains(low, "googleimageproxy"):
		return "Gmail (proxy)"
	case strings.Contains(low, "outlook") || strings.Contains(low, "microsoft office"):
		return "Outlook"
	case strings.Contains(low, "yahoo"):
		return "Yahoo Mail"
	case strings.Contains(low, "applemail") || strings.Contains(low, "apple mail"):
		return "Apple Mail"
	case strings.Contains(low, "edg/"):
		return "Edge"
	case strings.Contains(low, "chrome"):
		return "Chrome"
	case strings.Contains(low, "firefox"):
		return "Firefox"
	case strings.Contains(low, "safari"):
		return "Safari"
	default:
		return "other"
	}
}
