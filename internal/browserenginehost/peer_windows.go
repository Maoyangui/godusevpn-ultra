//go:build windows && (amd64 || arm64)

package browserenginehost

import (
	"encoding/binary"
	"io"
	"net"
	"os"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

var tcpTable = windows.NewLazySystemDLL("iphlpapi.dll").NewProc("GetExtendedTcpTable")
var isProcessInJob = windows.NewLazySystemDLL("kernel32.dll").NewProc("IsProcessInJob")

type processPin windows.Handle

func (p processPin) Close() error { return windows.CloseHandle(windows.Handle(p)) }

type processAuthorizer struct {
	process  *nativeProcess
	programs map[string]bool
	sid      string
}

func newProcessAuthorizer(process *nativeProcess, programs []string) (*processAuthorizer, error) {
	account, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil || account.User.Sid == nil {
		return nil, ErrRefused
	}
	allowed := map[string]bool{}
	for _, program := range programs {
		allowed[strings.ToLower(program)] = true
	}
	return &processAuthorizer{process, allowed, account.User.Sid.String()}, nil
}
func (a *processAuthorizer) authorize(connection net.Conn) (_ io.Closer, result error) {
	pid, err := peerPID(connection)
	if err != nil {
		return nil, ErrRefused
	}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, pid)
	if err != nil {
		return nil, ErrRefused
	}
	complete := false
	defer func() {
		if !complete {
			windows.CloseHandle(handle)
		}
	}()
	var code uint32
	var member int32
	success, _, _ := isProcessInJob.Call(uintptr(handle), uintptr(a.process.job), uintptr(unsafe.Pointer(&member)))
	if success == 0 || member == 0 || windows.GetExitCodeProcess(handle, &code) != nil || code != 259 {
		return nil, ErrRefused
	}
	var created, exit, kernel, user windows.Filetime
	if windows.GetProcessTimes(handle, &created, &exit, &kernel, &user) != nil || uint64(created.HighDateTime)<<32|uint64(created.LowDateTime) < a.process.created {
		return nil, ErrRefused
	}
	var image [32768]uint16
	length := uint32(len(image))
	if windows.QueryFullProcessImageName(handle, 0, &image[0], &length) != nil || !a.programs[strings.ToLower(windows.UTF16ToString(image[:length]))] {
		return nil, ErrRefused
	}
	var token windows.Token
	if windows.OpenProcessToken(handle, windows.TOKEN_QUERY, &token) != nil {
		return nil, ErrRefused
	}
	account, err := token.GetTokenUser()
	token.Close()
	if err != nil || account.User.Sid == nil || account.User.Sid.String() != a.sid {
		return nil, ErrRefused
	}
	actual, err := peerPID(connection)
	if err != nil || actual != pid || windows.GetExitCodeProcess(handle, &code) != nil || code != 259 {
		return nil, ErrRefused
	}
	complete = true
	return processPin(handle), nil
}
func ownBackend(connection net.Conn) error {
	pid, err := peerPID(connection)
	if err != nil || pid != uint32(os.Getpid()) {
		return ErrRefused
	}
	return nil
}
func peerPID(connection net.Conn) (uint32, error) {
	local, ok := connection.LocalAddr().(*net.TCPAddr)
	if !ok || !local.IP.Equal(net.IPv4(127, 0, 0, 1)) {
		return 0, ErrRefused
	}
	remote, ok := connection.RemoteAddr().(*net.TCPAddr)
	if !ok || !remote.IP.Equal(net.IPv4(127, 0, 0, 1)) {
		return 0, ErrRefused
	}
	var size uint32
	result, _, _ := tcpTable.Call(0, uintptr(unsafe.Pointer(&size)), 0, 2, 5, 0)
	if result != 122 || size < 4 || size > 16<<20 {
		return 0, ErrRefused
	}
	buffer := make([]byte, size)
	result, _, _ = tcpTable.Call(uintptr(unsafe.Pointer(&buffer[0])), uintptr(unsafe.Pointer(&size)), 0, 2, 5, 0)
	if result != 0 || size < 4 || int(size) > len(buffer) {
		return 0, ErrRefused
	}
	return reverseTuplePID(buffer[:size], remote.Port, local.Port)
}
func reverseTuplePID(table []byte, peerPort, ownPort int) (uint32, error) {
	if len(table) < 4 || peerPort < 1 || peerPort > 65535 || ownPort < 1 || ownPort > 65535 {
		return 0, ErrRefused
	}
	count := binary.LittleEndian.Uint32(table)
	if count > uint32((len(table)-4)/24) {
		return 0, ErrRefused
	}
	var pid uint32
	matches := 0
	port := func(value uint32) int {
		if value>>16 != 0 {
			return -1
		}
		return int(uint16(value&255)<<8 | uint16(value>>8&255))
	}
	for i := uint32(0); i < count; i++ {
		row := table[4+int(i)*24:]
		if binary.LittleEndian.Uint32(row) != 5 || binary.LittleEndian.Uint32(row[4:]) != 0x0100007f || port(binary.LittleEndian.Uint32(row[8:])) != peerPort || binary.LittleEndian.Uint32(row[12:]) != 0x0100007f || port(binary.LittleEndian.Uint32(row[16:])) != ownPort {
			continue
		}
		candidate := binary.LittleEndian.Uint32(row[20:])
		if candidate == 0 {
			return 0, ErrRefused
		}
		pid = candidate
		matches++
	}
	if matches != 1 {
		return 0, ErrRefused
	}
	return pid, nil
}
