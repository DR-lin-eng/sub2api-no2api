package service

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
)

const defaultBackupPartSizeBytes int64 = 4 * 1024 * 1024 * 1024

type backupPartPlan struct {
	Part   BackupPart
	Offset int64
}

// planBackupFileParts hashes contiguous ranges of one archive file. It does
// not materialize separate part files, keeping temporary disk usage bounded to
// the compressed archive itself.
func planBackupFileParts(srcPath string, partSize int64, partKey func(index int) string) ([]backupPartPlan, error) {
	if partSize <= 0 {
		return nil, errors.New("backup part size must be positive")
	}
	if partKey == nil {
		return nil, errors.New("backup part key builder is required")
	}

	src, err := os.Open(srcPath)
	if err != nil {
		return nil, fmt.Errorf("open backup archive: %w", err)
	}
	defer func() { _ = src.Close() }()

	info, err := src.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat backup archive: %w", err)
	}
	if info.Size() <= 0 {
		return nil, errors.New("backup archive is empty")
	}

	parts := make([]backupPartPlan, 0, (info.Size()+partSize-1)/partSize)
	for offset, index := int64(0), 1; offset < info.Size(); index++ {
		size := min(partSize, info.Size()-offset)
		hash := sha256.New()
		read, copyErr := io.Copy(hash, io.NewSectionReader(src, offset, size))
		if copyErr != nil {
			return nil, fmt.Errorf("hash backup part %d: %w", index, copyErr)
		}
		if read != size {
			return nil, fmt.Errorf("hash backup part %d: read %d bytes, want %d", index, read, size)
		}
		parts = append(parts, backupPartPlan{
			Part: BackupPart{
				Index: index, S3Key: partKey(index), SizeBytes: size,
				SHA256: hex.EncodeToString(hash.Sum(nil)),
			},
			Offset: offset,
		})
		offset += size
	}
	return parts, nil
}

func cleanupBackupFiles(paths ...string) error {
	var errs []error
	for _, path := range paths {
		if path == "" {
			continue
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, fmt.Errorf("remove %s: %w", path, err))
		}
	}
	return errors.Join(errs...)
}
