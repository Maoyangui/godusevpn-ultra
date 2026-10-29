//go:build windows

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/Maoyangui/godusevpn/internal/locationportal"
	"golang.org/x/sys/windows"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type locationConsumerPlan struct {
	path    string
	release func()
}

func inspectLocationConsumer() (locationConsumerPlan, error) {
	fail := func() (locationConsumerPlan, error) { return locationConsumerPlan{}, locationportal.ErrUnsupported }
	if ordinaryBrowserUser() != nil {
		return fail()
	}
	base, e := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, windows.KF_FLAG_DEFAULT)
	if e != nil {
		return fail()
	}
	root := filepath.Join(base, "GodUseVPN Ultra")
	self, e := os.Executable()
	if e != nil || !strings.EqualFold(filepath.Clean(self), filepath.Join(root, "godusevpn.exe")) {
		return fail()
	}
	volume := filepath.VolumeName(root)
	if len(volume) != 2 || windows.GetDriveType(windows.StringToUTF16Ptr(volume+`\`)) != windows.DRIVE_FIXED {
		return fail()
	}
	var held []*os.File
	files := map[string]*os.File{}
	release := func() {
		for i := len(held) - 1; i >= 0; i-- {
			held[i].Close()
		}
	}
	complete := false
	defer func() {
		if !complete {
			release()
		}
	}()
	pin := func(path string, directory, ancestor bool) error {
		access := uint32(windows.READ_CONTROL | windows.FILE_READ_ATTRIBUTES)
		share := uint32(windows.FILE_SHARE_READ)
		flags := uint32(windows.FILE_FLAG_OPEN_REPARSE_POINT)
		if directory {
			access |= windows.FILE_LIST_DIRECTORY
			share |= windows.FILE_SHARE_WRITE
			flags |= windows.FILE_FLAG_BACKUP_SEMANTICS
		} else {
			access |= windows.FILE_READ_DATA
		}
		h, e := windows.CreateFile(windows.StringToUTF16Ptr(path), access, share, nil, windows.OPEN_EXISTING, flags, 0)
		if e != nil {
			return locationportal.ErrUnsupported
		}
		f := os.NewFile(uintptr(h), "location-consumer-code")
		held = append(held, f)
		var info windows.ByHandleFileInformation
		if windows.GetFileInformationByHandle(h, &info) != nil || info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || (info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0) != directory || !directory && info.NumberOfLinks != 1 {
			return locationportal.ErrUnsupported
		}
		sd, e := windows.GetSecurityInfo(h, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
		if e != nil || browserCodeDescriptor(sd, ancestor) != nil {
			return locationportal.ErrUnsupported
		}
		if !directory {
			files[path] = f
		}
		return nil
	}
	current := volume + `\`
	if pin(current, true, true) != nil {
		return fail()
	}
	for _, part := range strings.Split(strings.TrimPrefix(root, current), `\`) {
		current = filepath.Join(current, part)
		if pin(current, true, current != root) != nil {
			return fail()
		}
	}
	manifest := filepath.Join(root, "SHA256SUMS")
	consumerRoot := filepath.Join(root, "location-consumer")
	exe := filepath.Join(consumerRoot, "GodUseVPN.Ultra.LocationConsumer.exe")
	if pin(self, false, false) != nil || pin(manifest, false, false) != nil || pin(consumerRoot, true, false) != nil {
		return fail()
	}
	entries, e := os.ReadDir(consumerRoot)
	if e != nil || len(entries) != 2 {
		return fail()
	}
	expectedLeaves := map[string]bool{"GodUseVPN.Ultra.LocationConsumer.exe": true, "GodUseVPN.Ultra.LocationConsumer.exe.config": true}
	for _, entry := range entries {
		if !expectedLeaves[entry.Name()] || entry.IsDir() || pin(filepath.Join(consumerRoot, entry.Name()), false, false) != nil {
			return fail()
		}
	}
	raw, e := io.ReadAll(io.LimitReader(files[manifest], (4<<20)+1))
	if e != nil || len(raw) > 4<<20 {
		return fail()
	}
	expected, e := browserManifestHashes(raw)
	if e != nil {
		return fail()
	}
	for path, file := range files {
		if path == manifest {
			continue
		}
		stat, e := file.Stat()
		if e != nil || stat.Size() > 32<<20 {
			return fail()
		}
		h := sha256.New()
		if _, e = io.Copy(h, io.LimitReader(file, (32<<20)+1)); e != nil {
			return fail()
		}
		relative, e := filepath.Rel(root, path)
		if e != nil || expected[strings.ToLower(filepath.ToSlash(relative))] != hex.EncodeToString(h.Sum(nil)) {
			return fail()
		}
	}
	complete = true
	return locationConsumerPlan{exe, release}, nil
}

type locationBoundedOutput struct {
	bytes.Buffer
	limit  int
	failed bool
}

func (w *locationBoundedOutput) Write(b []byte) (int, error) {
	if len(b) > w.limit-w.Len() {
		w.failed = true
		return 0, io.ErrShortWrite
	}
	return w.Buffer.Write(b)
}
func decodeLocationConsumer(raw []byte) (locationportal.Observation, error) {
	if !bytes.HasSuffix(raw, []byte("\n")) {
		return locationportal.Observation{}, locationportal.ErrReadback
	}
	return locationportal.DecodeObservation(raw)
}
func runLocationConsumer(ctx context.Context, request locationportal.ObserveRequest) (locationportal.Observation, error) {
	if ctx == nil || ctx.Err() != nil || request.TimeoutSeconds < 2 || request.TimeoutSeconds > 12 {
		return locationportal.Observation{}, locationportal.ErrReadback
	}
	plan, e := inspectLocationConsumer()
	if e != nil {
		return locationportal.Observation{}, e
	}
	defer plan.release()
	run, cancel := context.WithTimeout(ctx, 110*time.Second)
	defer cancel()
	frame := struct {
		SchemaVersion  int    `json:"schemaVersion"`
		RequestedAfter string `json:"requestedAfter"`
		TimeoutSeconds int    `json:"timeoutSeconds"`
	}{1, request.RequestedAfter, request.TimeoutSeconds}
	raw, e := json.Marshal(frame)
	if e != nil {
		return locationportal.Observation{}, locationportal.ErrInput
	}
	command := exec.CommandContext(run, plan.path)
	command.Dir = filepath.Dir(plan.path)
	command.Env = browserChildEnvironment(os.Environ())
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW}
	command.Stdin = bytes.NewReader(append(raw, '\n'))
	output := &locationBoundedOutput{limit: 4096}
	diagnostic := &locationBoundedOutput{limit: 1024}
	command.Stdout = output
	command.Stderr = diagnostic
	if command.Run() != nil || output.failed || diagnostic.failed || diagnostic.Len() != 0 {
		return locationportal.Observation{}, locationportal.ErrReadback
	}
	return decodeLocationConsumer(output.Bytes())
}
