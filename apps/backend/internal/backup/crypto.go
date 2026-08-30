package backup

import (
	"fmt"
	"io"

	"filippo.io/age"
)

const encSuffix = ".age" // so restore and retention can tell the formats apart

// age's STREAM construction chunks and authenticates, so a truncated or tampered object fails decryption instead of restoring quietly - the returned reader must be Closed by the caller even on upload failure, or the pump goroutine leaks blocked on the pipe
func encryptStream(passphrase string, src io.Reader) io.ReadCloser {
	pr, pw := io.Pipe()
	go func() {
		defer func() {
			if r := recover(); r != nil {
				pw.CloseWithError(fmt.Errorf("backup: encrypt pump panicked: %v", r))
			}
		}()
		rec, err := age.NewScryptRecipient(passphrase)
		if err != nil {
			pw.CloseWithError(fmt.Errorf("backup: scrypt recipient: %w", err))
			return
		}
		w, err := age.Encrypt(pw, rec)
		if err != nil {
			pw.CloseWithError(fmt.Errorf("backup: encrypt: %w", err))
			return
		}
		if _, err := io.Copy(w, src); err != nil {
			pw.CloseWithError(err)
			return
		}
		if err := w.Close(); err != nil {
			pw.CloseWithError(fmt.Errorf("backup: finalize encryption: %w", err))
			return
		}
		pw.Close()
	}()
	return pr
}

func decryptStream(passphrase string, src io.Reader) (io.Reader, error) {
	id, err := age.NewScryptIdentity(passphrase)
	if err != nil {
		return nil, fmt.Errorf("backup: scrypt identity: %w", err)
	}
	r, err := age.Decrypt(src, id)
	if err != nil {
		return nil, fmt.Errorf("backup: decrypt (wrong BACKUP_ENCRYPTION_PASSPHRASE?): %w", err)
	}
	return r, nil
}
