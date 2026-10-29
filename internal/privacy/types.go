// Package privacy manages opt-in local privacy policies. It has no dependency on
// the tunnel, daemon, routing, DNS, or connection state. A policy failure must
// never be used as a reason to stop or weaken a tunnel.
package privacy

import (
	"encoding/binary"
	"fmt"
)

type Report struct {
	Platform         string         `json:"platform"`
	Admin            bool           `json:"admin"`
	ObserverElevated bool           `json:"observerElevated"`
	Controls         []ControlState `json:"controls"`
	Capabilities     []Capability   `json:"capabilities"`
	Warnings         []string       `json:"warnings"`
	BackupHealthy    bool           `json:"backupHealthy"`
}

type ControlState struct {
	ID            string   `json:"id"`
	Title         string   `json:"title"`
	Category      string   `json:"category"`
	Scope         string   `json:"scope"`
	Description   string   `json:"description"`
	Impact        string   `json:"impact"`
	Status        string   `json:"status"`
	Detail        string   `json:"detail"`
	ReadStatus    string   `json:"readStatus"`
	Managed       bool     `json:"managed"`
	RequiresAdmin bool     `json:"requiresAdmin"`
	CanApply      bool     `json:"canApply"`
	Guide         string   `json:"guide"`
	SettingsID    string   `json:"settingsId,omitempty"`
	Restart       bool     `json:"restart"`
	Sources       []string `json:"sources"`
}

type Capability struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Scope       string `json:"scope"`
	Description string `json:"description"`
	Guide       string `json:"guide"`
	Status      string `json:"status"`
	SettingsID  string `json:"settingsId,omitempty"`
}

type ActionResult struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Message string `json:"message"`
	Changed bool   `json:"changed"`
}

type location struct {
	Hive string `json:"hive"`
	Path string `json:"path"`
	Name string `json:"name"`
}
type value struct {
	Exists bool   `json:"exists"`
	Type   uint32 `json:"type"`
	Data   []byte `json:"data,omitempty"`
}

func dword(v uint32) value {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, v)
	return value{true, 4, b}
}
func textValue(v string) value {
	// All catalog string values are ASCII. Windows REG_SZ is UTF-16LE.
	b := make([]byte, (len(v)+1)*2)
	for i := range v {
		binary.LittleEndian.PutUint16(b[i*2:], uint16(v[i]))
	}
	return value{true, 1, b}
}

type mutation struct {
	Location location
	Desired  value
}
type control struct {
	State        ControlState
	Changes      []mutation
	MinBuild     uint32
	SystemPolicy bool
}
type environment struct {
	Platform string
	Admin    bool
	SafeUser bool
	Edition  string
	Build    uint32
	UserKey  string // Current user SID; only a local binding, never included in Report.
	Warnings []string
}
type backend interface {
	environment() environment
	read(location) (value, error)
	write(location, value) error
}

func same(a, b value) bool {
	if a.Exists != b.Exists {
		return false
	}
	if !a.Exists {
		return true
	}
	if a.Type != b.Type || len(a.Data) != len(b.Data) {
		return false
	}
	for i := range a.Data {
		if a.Data[i] != b.Data[i] {
			return false
		}
	}
	return true
}
func (l location) key() string { return fmt.Sprintf("%s\\%s\\%s", l.Hive, l.Path, l.Name) }
