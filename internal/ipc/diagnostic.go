package ipc

// DiagnosticOptions is an explicit allowlist, never a collection preset.
// The zero value does not read logs, raw state/configuration or network tools.
type DiagnosticOptions struct {
	Logs                 bool `json:"logs"`
	Config               bool `json:"config"`
	Crashes              bool `json:"crashes"`
	Network              bool `json:"network"`
	AcknowledgeSensitive bool `json:"acknowledgeSensitive"`
}

type DiagnosticFile struct {
	Name    string `json:"name"`
	Content string `json:"content"`
	Bytes   int    `json:"bytes"`
}

type DiagnosticPreview struct {
	ID        string            `json:"id"`
	ExpiresAt string            `json:"expiresAt"`
	Selection DiagnosticOptions `json:"selection"`
	Files     []DiagnosticFile  `json:"files"`
	Warnings  []string          `json:"warnings"`
	SHA256    string            `json:"sha256"`
}

type DiagnosticExportRequest struct {
	PreviewID string `json:"previewId"`
}

// Data is base64 in JSON. It is delivered over the authenticated control
// channel; an ordinary GUI does not read private service files by path.
type DiagnosticArchive struct {
	Filename string `json:"filename"`
	Data     []byte `json:"data"`
	SHA256   string `json:"sha256"`
}

const (
	DiagnosticMaxFileBytes = 128 << 10
	DiagnosticMaxTextBytes = 2 << 20
	DiagnosticMaxZIPBytes  = 3 << 20 // Base64 payload is at most 4 MiB.
)
