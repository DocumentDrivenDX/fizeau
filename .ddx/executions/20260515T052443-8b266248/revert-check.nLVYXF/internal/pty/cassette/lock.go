package cassette

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/easel/fizeau/internal/safefs"
)

type RecordLock struct {
	path string
	file *os.File
}

func AcquireRecordLock(root, account string) (*RecordLock, error) {
	if account == "" {
		account = "default"
	}
	sum := sha256.Sum256([]byte(account))
	name := hex.EncodeToString(sum[:])
	dir := filepath.Join(root, "locks")
	if err := safefs.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, name+".lock")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600) // #nosec G304 -- lock path is under caller-selected cassette root.
	if errors.Is(err, os.ErrExist) {
		return nil, fmt.Errorf("cassette: record lock already held for account %q", account)
	}
	if err != nil {
		return nil, err
	}
	if _, err := file.WriteString(fmt.Sprintf("account=%s\n", account)); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return nil, err
	}
	return &RecordLock{path: path, file: file}, nil
}

func (l *RecordLock) Release() error {
	if l == nil {
		return nil
	}
	var err error
	if l.file != nil {
		err = l.file.Close()
		l.file = nil
	}
	if removeErr := safefs.Remove(l.path); removeErr != nil && err == nil {
		err = removeErr
	}
	return err
}
