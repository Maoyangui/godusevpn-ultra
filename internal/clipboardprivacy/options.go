package clipboardprivacy

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
)

type optionsFile struct {
	SchemaVersion int     `json:"schemaVersion"`
	Options       Options `json:"options"`
}

func LoadOptions(path string) (Options, error) {
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return DefaultOptions(), nil
	}
	if err != nil {
		return DefaultOptions(), ErrUnavailable
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 4097))
	if err != nil || len(data) > 4096 {
		return DefaultOptions(), ErrInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var stored optionsFile
	if decoder.Decode(&stored) != nil || stored.SchemaVersion != 1 || stored.Options.Validate() != nil {
		return DefaultOptions(), ErrInvalid
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return DefaultOptions(), ErrInvalid
	}
	return stored.Options, nil
}
func SaveOptions(path string, options Options) error {
	if options.Validate() != nil {
		return ErrInvalid
	}
	data, err := json.Marshal(optionsFile{1, options})
	if err != nil {
		return ErrInvalid
	}
	if os.MkdirAll(filepath.Dir(path), 0700) != nil {
		return ErrUnavailable
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".clipboard-options-*")
	if err != nil {
		return ErrUnavailable
	}
	defer os.Remove(file.Name())
	if _, err = file.Write(data); err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return ErrUnavailable
	}
	if os.Rename(file.Name(), path) != nil {
		return ErrUnavailable
	}
	return nil
}
