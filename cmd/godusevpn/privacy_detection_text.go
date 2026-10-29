package main

import (
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/Maoyangui/godusevpn/internal/detectiontext"
)

var errPrivacyDetectionText = errors.New("检测精简预览已过期、失效或改变；请重新生成并审阅")

type PrivacyDetectionTextView struct {
	PreviewID string `json:"previewId"`
	Content   string `json:"content"`
	SHA256    string `json:"sha256"`
	ExpiresAt string `json:"expiresAt"`
	Count     int    `json:"count"`
}

// reviewedPrivacyText derives only the immutable, backend-held export bytes.
// Existing privacy epoch invalidation clears both pointers checked here.
func reviewedPrivacyText(preview *privacyExportSnapshot, snapshot *privacyDetectionSnapshot, id string, now time.Time) (detectiontext.Preview, error) {
	if len(id) != 48 || preview == nil || snapshot == nil || preview.id != id || preview.sourceID != snapshot.id || !now.Before(preview.expires) || !now.Before(snapshot.expires) {
		return detectiontext.Preview{}, errPrivacyDetectionText
	}
	if _, err := hex.DecodeString(id); err != nil || strings.ToLower(id) != id {
		return detectiontext.Preview{}, errPrivacyDetectionText
	}
	result, err := detectiontext.FromReviewed(preview.content)
	if err != nil {
		return detectiontext.Preview{}, errPrivacyDetectionText
	}
	return result, nil
}

// PrivacyDetectionTextPreview does not replace the JSON preview or generate a
// new observation. The UI shows this content before offering an explicit copy.
func (a *App) PrivacyDetectionTextPreview(previewID string) (PrivacyDetectionTextView, error) {
	a.privacyMu.Lock()
	defer a.privacyMu.Unlock()
	result, err := reviewedPrivacyText(a.privacyExport, a.privacySnapshot, previewID, time.Now())
	if err != nil {
		return PrivacyDetectionTextView{}, err
	}
	return PrivacyDetectionTextView{PreviewID: previewID, Content: result.Content, SHA256: result.SHA256, ExpiresAt: a.privacyExport.expires.UTC().Format(time.RFC3339), Count: result.Count}, nil
}

// PrivacyDetectionTextForCopy returns exactly the text the user just reviewed.
// Clipboard access stays in the existing explicit frontend copy operation.
func (a *App) PrivacyDetectionTextForCopy(previewID, expectedSHA256 string) (string, error) {
	if len(expectedSHA256) != 64 || strings.ToLower(expectedSHA256) != expectedSHA256 {
		return "", errPrivacyDetectionText
	}
	if _, err := hex.DecodeString(expectedSHA256); err != nil {
		return "", errPrivacyDetectionText
	}
	a.privacyMu.Lock()
	defer a.privacyMu.Unlock()
	result, err := reviewedPrivacyText(a.privacyExport, a.privacySnapshot, previewID, time.Now())
	if err != nil || subtle.ConstantTimeCompare([]byte(result.SHA256), []byte(expectedSHA256)) != 1 {
		return "", errPrivacyDetectionText
	}
	return result.Content, nil
}
