package privacydata

import (
	"errors"
	"io"
	"os"
)

// WriteNewLocal writes an explicitly chosen report to a new ordinary local
// file. Reports use a private ACL from creation on Windows, share image exports'
// path/handle verification, and never overwrite an existing file.
func WriteNewLocal(path string, content []byte) error {
	return writeNewLocalUsing(path, content, openPrivateDestination)
}

func writeNewLocalUsing(path string, content []byte, create func(string) (*os.File, func() error, error)) (err error) {
	if len(content) > 4<<20 {
		return errors.New("报告超过本地保存上限")
	}
	release, err := pinValidatedPath(path, true)
	if err != nil {
		return err
	}
	defer release()
	f, discard, err := create(path)
	if err != nil {
		return err
	}
	complete := false
	defer func() {
		if !complete {
			if e := discard(); e != nil {
				err = errors.Join(err, errors.New("未完成的报告无法清理，请检查所选文件"))
			}
		}
		if e := f.Close(); e != nil {
			err = errors.Join(err, errors.New("报告关闭失败，请核对所选文件"))
		}
	}()
	n, err := f.Write(content)
	if err == nil && n != len(content) {
		err = io.ErrShortWrite
	}
	if err != nil {
		return errors.New("报告写入失败")
	}
	if err = f.Sync(); err != nil {
		return errors.New("报告无法持久保存")
	}
	complete = true
	return nil
}
