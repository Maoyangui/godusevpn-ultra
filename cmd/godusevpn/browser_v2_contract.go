package main

import "github.com/Maoyangui/godusevpn/internal/browserprofiles"

// BrowserProfileV2PreviewResult keeps the capability receipt beside the
// legacy preview payload. A caller can therefore distinguish a rejected
// field from a host failure without guessing from a localized error string.
type BrowserProfileV2PreviewResult struct {
	Status  string                            `json:"status"`
	Detail  string                            `json:"detail"`
	Receipt browserprofiles.CapabilityReceipt `json:"receipt"`
	Preview *BrowserProfilePreviewResult      `json:"preview,omitempty"`
}

type BrowserProfileV2LaunchResult struct {
	Status  string                            `json:"status"`
	Detail  string                            `json:"detail"`
	Receipt browserprofiles.CapabilityReceipt `json:"receipt"`
	Launch  *BrowserProfileLaunchResult       `json:"launch,omitempty"`
}

func parseBrowserProfileV2Request(data string) (browserprofiles.LaunchRequest, browserprofiles.CapabilityReceipt, error) {
	return browserprofiles.ParseLaunchRequest([]byte(data))
}

func browserProfileV2Unsupported(receipt browserprofiles.CapabilityReceipt, fields ...string) browserprofiles.CapabilityReceipt {
	receipt.Accepted = false
	seen := make(map[string]bool, len(receipt.Unsupported)+len(fields))
	for _, field := range receipt.Unsupported {
		seen[field] = true
	}
	for _, field := range fields {
		if !seen[field] {
			receipt.Unsupported = append(receipt.Unsupported, field)
			seen[field] = true
		}
	}
	return receipt
}
