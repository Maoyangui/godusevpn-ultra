//go:build windows

package timeprivacy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

type windowsBackend struct{}

func newBackend() backend { return windowsBackend{} }

type dynamicZone struct {
	Bias                        int32
	StandardName                [32]uint16
	StandardDate                windows.Systemtime
	StandardBias                int32
	DaylightName                [32]uint16
	DaylightDate                windows.Systemtime
	DaylightBias                int32
	TimeZoneKeyName             [128]uint16
	DynamicDaylightTimeDisabled uint8
}

var enumZone = windows.NewLazySystemDLL("advapi32.dll").NewProc("EnumDynamicTimeZoneInformation")
var getZone = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetDynamicTimeZoneInformation")

func (windowsBackend) supported() bool { return enumZone.Find() == nil && getZone.Find() == nil }
func (windowsBackend) choices() ([]Choice, error) {
	if enumZone.Find() != nil {
		return nil, errors.New("time-zone enumeration unavailable")
	}
	out := []Choice{}
	for index := uint32(0); index < 1024; index++ {
		var zone dynamicZone
		status, _, _ := enumZone.Call(uintptr(index), uintptr(unsafe.Pointer(&zone)))
		if status == uintptr(windows.ERROR_NO_MORE_ITEMS) {
			return out, nil
		}
		if status != 0 {
			return nil, errors.New("time-zone enumeration failed")
		}
		id := windows.UTF16ToString(zone.TimeZoneKeyName[:])
		if id == "" {
			return nil, errors.New("empty system time-zone ID")
		}
		minutes := -(zone.Bias + zone.StandardBias)
		sign := "+"
		if minutes < 0 {
			sign = "-"
			minutes = -minutes
		}
		label := fmt.Sprintf("(UTC%s%02d:%02d) %s", sign, minutes/60, minutes%60, windows.UTF16ToString(zone.StandardName[:]))
		out = append(out, Choice{id, label})
	}
	return nil, errors.New("system time-zone catalog exceeded limit")
}
func (windowsBackend) current() (zoneState, error) {
	if getZone.Find() != nil {
		return zoneState{}, errors.New("current time-zone API unavailable")
	}
	var zone dynamicZone
	status, _, _ := getZone.Call(uintptr(unsafe.Pointer(&zone)))
	if uint32(status) == 0xffffffff {
		return zoneState{}, errors.New("current time zone unavailable")
	}
	id := windows.UTF16ToString(zone.TimeZoneKeyName[:])
	if id == "" {
		return zoneState{}, errors.New("custom time zone cannot be restored by ID")
	}
	return zoneState{id, zone.DynamicDaylightTimeDisabled != 0}, nil
}
func (b windowsBackend) set(state zoneState) error {
	choices, err := b.choices()
	if err != nil || !listed(choices, state.ID) {
		return errors.New("time-zone ID is not in Windows catalog")
	}
	systemDir, err := windows.GetSystemDirectory()
	if err != nil || !filepath.IsAbs(systemDir) {
		return errors.New("cannot resolve trusted system directory")
	}
	arg := state.ID
	if state.DSTDisabled {
		arg += "_dstoff"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	// No shell, PATH lookup, arbitrary flags, location lookup, or UTC clock API.
	cmd := exec.CommandContext(ctx, filepath.Join(systemDir, "tzutil.exe"), "/s", arg)
	cmd.Dir = systemDir
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if cmd.Run() != nil {
		return errors.New("timezone change denied or not confirmed")
	}
	return nil
}
