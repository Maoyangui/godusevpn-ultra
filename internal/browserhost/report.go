// Package browserhost describes the browser engines that Ultra can safely
// inspect on the current machine. It deliberately does not change browser
// policy, launch a browser, or pretend that the UI WebView is a private
// browsing profile.
package browserhost

// Engine is a locally discovered browser runtime. A missing runtime is
// represented by an Engine with Available=false so the UI can explain the
// boundary without displaying a misleading switch.
type Engine struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Version   string `json:"version,omitempty"`
	Available bool   `json:"available"`
	Detail    string `json:"detail"`
}

// Report is a read-only inventory. It intentionally omits executable paths,
// user-data paths, profile names and command lines because those can identify
// the machine or user and are not needed to decide whether an engine can be
// offered.
type Report struct {
	Supported   bool     `json:"supported"`
	Platform    string   `json:"platform"`
	Engines     []Engine `json:"engines"`
	Recommended string   `json:"recommended,omitempty"`
	Warnings    []string `json:"warnings,omitempty"`
}
