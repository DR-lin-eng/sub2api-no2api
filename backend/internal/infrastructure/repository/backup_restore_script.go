package repository

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// Stage the complete decompressed input before executing it, so read/checksum
// errors cannot commit a truncated restore. Only pg_dump's outer restriction
// envelope is removed; SQL, COPY data and all interior commands stay verbatim.
func prepareBackupRestore(ctx context.Context, data io.Reader) (io.Reader, func(), error) {
	file, err := os.CreateTemp("", "sub2api-restore-*.sql")
	if err != nil {
		return nil, nil, fmt.Errorf("stage restore: %w", err)
	}
	cleanup := func() { _ = file.Close(); _ = os.Remove(file.Name()) }
	size, err := io.Copy(file, backupRestoreReader{ctx: ctx, reader: data})
	if err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("read restore: %w", err)
	}
	script, err := backupRestoreSection(file, size)
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	return script, cleanup, nil
}

type backupRestoreReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r backupRestoreReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

func backupRestoreSection(file *os.File, size int64) (io.Reader, error) {
	const headerLimit = 64 * 1024
	whole := io.NewSectionReader(file, 0, size)
	reader := bufio.NewReaderSize(whole, headerLimit)
	var start int64
	var key string
	for start < headerLimit {
		line, err := reader.ReadSlice('\n')
		if errors.Is(err, bufio.ErrBufferFull) {
			break
		}
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("read restore header: %w", err)
		}
		start += int64(len(line))
		text := strings.TrimSpace(string(line))
		if text == "" || strings.HasPrefix(text, "--") {
			if errors.Is(err, io.EOF) {
				break
			}
			continue
		}
		fields := strings.Fields(text)
		if fields[0] != "\\restrict" {
			break
		}
		if len(fields) != 2 || !backupRestrictionKey(fields[1]) {
			return nil, fmt.Errorf("invalid restore restriction header")
		}
		key = fields[1]
		break
	}
	if key == "" {
		return io.NewSectionReader(file, 0, size), nil
	}
	// pg_dump writes its matching unrestrict as the last nonblank line. Never
	// remove interior markers: they must encounter the independent psql guard.
	tailSize := min(size, int64(4096))
	tail := make([]byte, tailSize)
	if _, err := file.ReadAt(tail, size-tailSize); err != nil {
		return nil, fmt.Errorf("read restore footer: %w", err)
	}
	trimmed := strings.TrimRight(string(tail), " \t\r\n")
	lastLine := strings.LastIndexByte(trimmed, '\n') + 1
	fields := strings.Fields(trimmed[lastLine:])
	end := size - tailSize + int64(lastLine)
	if len(fields) != 2 || fields[0] != "\\unrestrict" || fields[1] != key || end < start {
		return nil, fmt.Errorf("invalid restore restriction footer")
	}
	return io.NewSectionReader(file, start, end-start), nil
}

func backupRestrictionKey(key string) bool {
	if len(key) == 0 || len(key) > 256 {
		return false
	}
	for _, char := range key {
		if !(char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9') {
			return false
		}
	}
	return true
}
