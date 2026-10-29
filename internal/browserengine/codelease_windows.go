//go:build windows

package browserengine

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/Maoyangui/godusevpn/internal/localstate"
	"golang.org/x/sys/windows"
)

func pinCodeTree(instanceRoot, id, version, instanceID string, receipt Receipt) (_ *CodeLease, result error) {
	directory := filepath.Join(instanceRoot, id, version)
	parents, err := localstate.AcquireDirectory(directory, false)
	if err != nil {
		return nil, ErrStore
	}
	var held []*os.File
	release := func() {
		for i := len(held) - 1; i >= 0; i-- {
			held[i].Close()
		}
		parents()
	}
	complete := false
	defer func() {
		if !complete {
			release()
		}
	}()
	expected := map[string]TreeFile{}
	dirs := map[string]bool{".": true}
	for _, entry := range receipt.Files {
		if !safePath(entry.Name) || !hashValid(entry.SHA256, 32) || expected[entry.Name].Name != "" {
			return nil, ErrIntegrity
		}
		expected[entry.Name] = entry
	}
	for _, name := range receipt.Directories {
		if !safePath(name) || dirs[name] {
			return nil, ErrIntegrity
		}
		dirs[name] = true
	}
	files, dirCount := 0, 0
	var programs []string
	err = filepath.WalkDir(directory, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return ErrIntegrity
		}
		relative, err := filepath.Rel(directory, name)
		if err != nil {
			return ErrIntegrity
		}
		relative = filepath.ToSlash(relative)
		isDir := entry.IsDir()
		if isDir {
			if !dirs[relative] {
				return ErrIntegrity
			}
			dirCount++
		} else if relative != receiptName {
			if expected[relative].Name == "" {
				return ErrIntegrity
			}
		}
		access, flags := uint32(windows.GENERIC_READ|windows.READ_CONTROL), uint32(windows.FILE_FLAG_OPEN_REPARSE_POINT)
		if isDir {
			access = windows.FILE_LIST_DIRECTORY | windows.FILE_READ_ATTRIBUTES | windows.READ_CONTROL
			flags |= windows.FILE_FLAG_BACKUP_SEMANTICS
		}
		h, err := windows.CreateFile(windows.StringToUTF16Ptr(name), access, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, flags, 0)
		if err != nil {
			return ErrIntegrity
		}
		file := os.NewFile(uintptr(h), "ultra-verified-engine-code")
		held = append(held, file)
		var info windows.ByHandleFileInformation
		if windows.GetFileInformationByHandle(h, &info) != nil || info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || (info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0) != isDir || !isDir && info.NumberOfLinks != 1 {
			return ErrIntegrity
		}
		var final, long [32768]uint16
		n, err := windows.GetFinalPathNameByHandle(h, &final[0], uint32(len(final)), 0)
		if err != nil || n >= uint32(len(final)) {
			return ErrIntegrity
		}
		length, err := windows.GetLongPathName(windows.StringToUTF16Ptr(name), &long[0], uint32(len(long)))
		if err != nil || length >= uint32(len(long)) || !strings.EqualFold(strings.TrimPrefix(windows.UTF16ToString(final[:n]), `\\?\`), windows.UTF16ToString(long[:length])) {
			return ErrIntegrity
		}
		if isDir || relative == receiptName {
			return nil
		}
		want := expected[relative]
		stat, err := file.Stat()
		if err != nil || stat.Size() != want.Size {
			return ErrIntegrity
		}
		hash := sha256.New()
		if _, err = io.Copy(hash, io.LimitReader(file, want.Size+1)); err != nil || hex.EncodeToString(hash.Sum(nil)) != want.SHA256 {
			return ErrIntegrity
		}
		files++
		if strings.EqualFold(filepath.Ext(relative), ".exe") {
			programs = append(programs, name)
		}
		return nil
	})
	if err != nil || files != len(expected) || dirCount != len(dirs) || len(programs) == 0 {
		return nil, ErrIntegrity
	}
	complete = true
	return &CodeLease{instanceID: instanceID, directory: directory, mainPath: filepath.Join(directory, receipt.Executable), programs: programs, release: release}, nil
}
