package timeprivacy

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Maoyangui/godusevpn/internal/localstate"
)

const journalName = "timezone-journal.json"

type transaction struct {
	Before   zoneState `json:"before"`
	Previous zoneState `json:"previous"`
	Desired  zoneState `json:"desired"`
	Stage    string    `json:"stage"`
}
type stored struct {
	Version     int          `json:"version"`
	Transaction *transaction `json:"transaction"`
}
type envelope struct {
	Payload json.RawMessage `json:"payload"`
	SHA256  string          `json:"sha256"`
}

func (m *Manager) load() (*transaction, error) {
	return m.loadUsing(func(path string) (*os.File, error) { return localstate.OpenRegular(path, false) })
}

func (m *Manager) loadUsing(open func(string) (*os.File, error)) (*transaction, error) {
	release, err := localstate.AcquireDirectory(m.dir, false)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.New("时区恢复存储不是可确认的本地目录；未读取备份，仍可读取当前时区，自动修改和恢复已停用")
	}
	defer release()
	path := filepath.Join(m.dir, journalName)
	f, err := open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.New("时区备份无法读取，停止所有修改")
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 16385))
	if err != nil || len(b) > 16384 {
		return nil, errors.New("时区备份大小或读取异常，停止所有修改")
	}
	var e envelope
	if strict(b, &e) != nil {
		return nil, errors.New("时区备份格式损坏，停止修改；请保留备份")
	}
	sum := sha256.Sum256(e.Payload)
	if e.SHA256 != hex.EncodeToString(sum[:]) {
		return nil, errors.New("时区备份完整性校验失败，停止修改；请保留备份")
	}
	var s stored
	if strict(e.Payload, &s) != nil || s.Version != 1 {
		return nil, errors.New("时区备份版本或内容不符合要求，停止修改")
	}
	if j := s.Transaction; j != nil {
		if j.Stage != "applying" && j.Stage != "active" && j.Stage != "restoring" {
			return nil, errors.New("时区备份阶段不合法，停止修改")
		}
		for _, state := range []zoneState{j.Before, j.Previous, j.Desired} {
			if state.ID == "" || len(state.ID) > 127 || strings.ContainsAny(state.ID, "\x00\r\n\"\\/") {
				return nil, errors.New("时区备份 ID 不合法，停止修改")
			}
		}
	}
	return s.Transaction, nil
}
func strict(b []byte, v any) error {
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
func (m *Manager) save(j *transaction) error {
	p, err := json.Marshal(stored{1, j})
	if err != nil {
		return errors.New("时区备份编码失败")
	}
	sum := sha256.Sum256(p)
	b, err := json.Marshal(envelope{p, hex.EncodeToString(sum[:])})
	if err != nil {
		return errors.New("时区备份编码失败")
	}
	if m.persist(filepath.Join(m.dir, journalName), b) != nil {
		return errors.New("时区备份无法持久保存，已停止后续修改；请检查目录和磁盘空间")
	}
	return nil
}
func durableWrite(path string, b []byte) error {
	release, err := localstate.AcquireDirectory(filepath.Dir(path), true)
	if err != nil {
		return err
	}
	defer release()
	if err := secureDirectory(filepath.Dir(path)); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".timezone-journal-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(b)
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
	return replace(name, path)
}
