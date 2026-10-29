// This fixture exports static production help only. It does not construct an
// app, read the system, call a native API, open storage, or request the network.
package main

import (
	"encoding/json"
	"os"

	"github.com/Maoyangui/godusevpn/internal/centerguide"
)

func main() {
	if err := json.NewEncoder(os.Stdout).Encode(centerguide.Guides()); err != nil {
		os.Exit(1)
	}
}
