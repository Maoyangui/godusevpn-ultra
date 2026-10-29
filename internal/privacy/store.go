package privacy

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Maoyangui/godusevpn/internal/localstate"
)

const journalName = "privacy-journal.json"
const maxJournalSize = 4 << 20

type savedChange struct {
	Location location `json:"location"`
	Before   value    `json:"before"`
	Desired  value    `json:"desired"`
}
type savedControl struct {
	Stage   string        `json:"stage"` // applying, active, restoring: all recoverable.
	Changes []savedChange `json:"changes"`
}
type journal struct {
	Version  int                     `json:"version"`
	UserKey  string                  `json:"userKey"`
	Controls map[string]savedControl `json:"controls"`
}
type envelope struct {
	Payload json.RawMessage `json:"payload"`
	SHA256  string          `json:"sha256"`
}

func (m *Manager) load() (journal, error) {
	j := journal{Version: 1, UserKey: m.owner(), Controls: map[string]savedControl{}}
	if m.machineOnly && !machineContext(m.backend.environment()) {
		return j, errors.New("机器备份仅允许已提升的 Windows 进程读取；未访问用户配置")
	}
	if !m.machineOnly {
		release, err := localstate.AcquireDirectory(m.dir, false)
		if errors.Is(err, os.ErrNotExist) {
			return j, nil
		}
		if err != nil {
			return j, errors.New("用户恢复存储不是可确认的本地目录；未读取备份，仍可检查安全只读的策略值，自动修改和恢复已停用")
		}
		defer release()
	}
	if m.checkStore != nil {
		if err := m.checkStore(m.dir); err != nil {
			return j, errors.New("机器备份目录或权限无法确认；保留备份并停止策略操作")
		}
	}
	path := filepath.Join(m.dir, journalName)
	open := m.openJournal
	if open == nil {
		open = openUserJournal
	}
	f, err := open(path)
	if errors.Is(err, os.ErrNotExist) {
		return j, nil
	}
	if err != nil {
		return j, fmt.Errorf("隐私备份无法读取；停止策略写入")
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxJournalSize+1))
	if err != nil || len(b) > maxJournalSize {
		return j, errors.New("隐私备份大小或读取异常；停止策略写入")
	}
	var e envelope
	if err = decodeStrict(b, &e); err != nil {
		return j, errors.New("隐私备份格式损坏；停止策略写入，请保留原文件")
	}
	sum := sha256.Sum256(e.Payload)
	if e.SHA256 != hex.EncodeToString(sum[:]) {
		return j, errors.New("隐私备份完整性校验失败；停止策略写入，请保留原文件")
	}
	if err = decodeStrict(e.Payload, &j); err != nil {
		return j, errors.New("隐私备份内容损坏；停止策略写入")
	}
	if j.Version != 1 || j.Controls == nil || j.UserKey != m.owner() {
		return j, errors.New("隐私备份版本或当前用户不匹配；停止策略写入")
	}
	for id, s := range j.Controls {
		c, ok := m.control(id)
		if !ok || (s.Stage != "applying" && s.Stage != "active" && s.Stage != "restoring") || len(s.Changes) == 0 || len(s.Changes) > len(c.Changes) {
			return j, errors.New("隐私备份不符合固定策略目录；停止策略写入")
		}
		seen := map[string]bool{}
		for _, x := range s.Changes {
			valid := false
			for _, known := range c.Changes {
				if known.Location == x.Location && same(known.Desired, x.Desired) {
					valid = true
				}
			}
			if !valid || seen[x.Location.key()] || len(x.Before.Data) > 65536 || !validValue(x.Before) {
				return j, errors.New("隐私备份含非法策略或原值；停止策略写入")
			}
			seen[x.Location.key()] = true
		}
	}
	return j, nil
}

func openUserJournal(path string) (*os.File, error) {
	return localstate.OpenRegular(path, false)
}
func decodeStrict(b []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return err
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return errors.New("trailing content")
	}
	return nil
}
func validValue(v value) bool {
	if !v.Exists {
		return v.Type == 0 && len(v.Data) == 0
	}
	switch v.Type {
	case 4:
		return len(v.Data) == 4
	case 1, 2:
		n := len(v.Data)
		return n >= 2 && n%2 == 0 && v.Data[n-1] == 0 && v.Data[n-2] == 0
	case 7:
		n := len(v.Data)
		return n >= 4 && n%2 == 0 && v.Data[n-1] == 0 && v.Data[n-2] == 0 && v.Data[n-3] == 0 && v.Data[n-4] == 0
	case 3:
		return true
	case 11:
		return len(v.Data) == 8
	}
	return false
}
func (m *Manager) save(j journal) error {
	p, err := json.Marshal(j)
	if err != nil {
		return errors.New("隐私备份编码失败")
	}
	sum := sha256.Sum256(p)
	b, err := json.Marshal(envelope{Payload: p, SHA256: hex.EncodeToString(sum[:])})
	if err != nil {
		return errors.New("隐私备份编码失败")
	}
	if err = m.persist(filepath.Join(m.dir, journalName), b); err != nil {
		return errors.New("隐私备份无法持久保存；未开始的新写入已停止，请保留备份并检查目录权限和磁盘空间")
	}
	return nil
}

// Atomic replacement leaves either the old complete journal or the new complete
// journal. Sync happens before any corresponding registry mutation.
func durableWrite(path string, data []byte) error {
	release, err := localstate.AcquireDirectory(filepath.Dir(path), true)
	if err != nil {
		return err
	}
	defer release()
	if err := secureDirectory(filepath.Dir(path)); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".privacy-journal-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return atomicReplace(name, path)
}
