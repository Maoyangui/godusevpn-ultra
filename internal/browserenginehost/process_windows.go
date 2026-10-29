//go:build windows && (amd64 || arm64)

package browserenginehost

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

type nativeProcess struct {
	job, process, thread windows.Handle
	pid                  uint32
	created              uint64
	read, write          *os.File
	arguments            []string
	done                 chan struct{}
}

func (p *nativeProcess) resume() error {
	previous, err := windows.ResumeThread(p.thread)
	if err != nil || previous != 1 {
		return ErrRefused
	}
	windows.CloseHandle(p.thread)
	p.thread = 0
	return nil
}
func (p *nativeProcess) active() (uint32, error) {
	var info struct {
		user, kernel, periodUser, periodKernel int64
		faults, total, active, terminated      uint32
	}
	var length uint32
	if windows.QueryInformationJobObject(p.job, windows.JobObjectBasicAccountingInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)), &length) != nil || length != uint32(unsafe.Sizeof(info)) {
		return 0, ErrRefused
	}
	return info.active, nil
}
func (p *nativeProcess) terminateAndWait(ctx context.Context) error {
	if windows.TerminateJobObject(p.job, 1) != nil {
		return ErrRefused
	}
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		count, err := p.active()
		if err != nil {
			return err
		}
		if count == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return ErrRefused
		case <-ticker.C:
		}
	}
}
func (p *nativeProcess) close() {
	if p.thread != 0 {
		windows.CloseHandle(p.thread)
	}
	if p.read != nil {
		p.read.Close()
	}
	if p.write != nil {
		p.write.Close()
	}
	windows.CloseHandle(p.process)
	windows.CloseHandle(p.job)
}
func processEnvironment(timezone string) []uint16 {
	allowed := map[string]bool{"SYSTEMROOT": true, "WINDIR": true, "TEMP": true, "TMP": true, "USERPROFILE": true, "LOCALAPPDATA": true, "APPDATA": true, "PROGRAMDATA": true, "PROGRAMFILES": true, "PROGRAMFILES(X86)": true}
	var values []string
	for _, value := range os.Environ() {
		key, _, ok := strings.Cut(value, "=")
		if ok && allowed[strings.ToUpper(key)] {
			values = append(values, value)
		}
	}
	values = append(values, "TZ="+timezone)
	sortNames(values)
	var result []uint16
	for _, value := range values {
		encoded, _ := windows.UTF16FromString(value)
		result = append(result, encoded...)
	}
	return append(result, 0)
}
func launchSuspended(c Config, executable, profile string, relayPort, controlPort int) (launched *nativeProcess, result error) {
	process := &nativeProcess{done: make(chan struct{})}
	complete := false
	defer func() {
		if !complete {
			if process.process != 0 {
				// Return even an incompletely queried suspended child to its
				// lifetime owner. Cleanup must verify an empty Job before dropping
				// code pins; a fire-and-forget TerminateProcess is insufficient.
				launched = process
				return
			}
			process.close()
		}
	}()
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, ErrRefused
	}
	process.job = job
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		return nil, ErrRefused
	}
	attributes, err := windows.NewProcThreadAttributeList(2)
	if err != nil {
		return nil, ErrRefused
	}
	defer attributes.Delete()
	jobs := []windows.Handle{job}
	// PROC_THREAD_ATTRIBUTE_JOB_LIST joins the Job during CreateProcess itself.
	// The process never has a runnable interval outside its kill-on-close Job.
	if attributes.Update(0x0002000d, unsafe.Pointer(&jobs[0]), unsafe.Sizeof(jobs[0])) != nil {
		return nil, ErrRefused
	}
	var inherit []windows.Handle
	var childRead, childWrite *os.File
	if c.EngineID == "chrome-for-testing" {
		inRead, inWrite, err := os.Pipe()
		if err != nil {
			return nil, ErrRefused
		}
		outRead, outWrite, err := os.Pipe()
		if err != nil {
			inRead.Close()
			inWrite.Close()
			return nil, ErrRefused
		}
		childRead, childWrite = inRead, outWrite
		process.read, process.write = outRead, inWrite
		defer childRead.Close()
		defer childWrite.Close()
		inherit = []windows.Handle{windows.Handle(childRead.Fd()), windows.Handle(childWrite.Fd())}
		for _, handle := range inherit {
			if windows.SetHandleInformation(handle, windows.HANDLE_FLAG_INHERIT, windows.HANDLE_FLAG_INHERIT) != nil {
				return nil, ErrRefused
			}
		}
	}
	if len(inherit) > 0 && attributes.Update(windows.PROC_THREAD_ATTRIBUTE_HANDLE_LIST, unsafe.Pointer(&inherit[0]), uintptr(len(inherit))*unsafe.Sizeof(inherit[0])) != nil {
		return nil, ErrRefused
	}
	startup := windows.StartupInfoEx{StartupInfo: windows.StartupInfo{Cb: uint32(unsafe.Sizeof(windows.StartupInfoEx{}))}, ProcThreadAttributeList: attributes.List()}
	var in, out uintptr
	if childRead != nil {
		in = childRead.Fd()
		out = childWrite.Fd()
	}
	process.arguments = arguments(c, profile, relayPort, controlPort, in, out)
	args := append([]string{executable}, process.arguments...)
	line, err := windows.UTF16FromString(windows.ComposeCommandLine(args))
	if err != nil {
		return nil, ErrRefused
	}
	app, err := windows.UTF16PtrFromString(executable)
	if err != nil {
		return nil, ErrRefused
	}
	cwd := windows.StringToUTF16Ptr(filepath.Dir(executable))
	environment := processEnvironment(c.Timezone)
	var info windows.ProcessInformation
	flags := uint32(windows.CREATE_SUSPENDED | windows.CREATE_UNICODE_ENVIRONMENT | windows.EXTENDED_STARTUPINFO_PRESENT | windows.CREATE_DEFAULT_ERROR_MODE)
	if windows.CreateProcess(app, &line[0], nil, nil, len(inherit) > 0, flags, &environment[0], cwd, &startup.StartupInfo, &info) != nil {
		return nil, ErrRefused
	}
	runtime.KeepAlive(environment)
	runtime.KeepAlive(jobs)
	runtime.KeepAlive(inherit)
	process.process, process.thread, process.pid = info.Process, info.Thread, info.ProcessId
	var created, exit, kernel, user windows.Filetime
	if windows.GetProcessTimes(info.Process, &created, &exit, &kernel, &user) != nil {
		return nil, ErrRefused
	}
	process.created = uint64(created.HighDateTime)<<32 | uint64(created.LowDateTime)
	complete = true
	return process, nil
}
