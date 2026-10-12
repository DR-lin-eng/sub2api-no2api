package repository

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBackupRestoreScriptPreservesSQLAndLargeCopyRows(t *testing.T) {
	inner := "CREATE TABLE example(value text);\r\nCOPY example FROM stdin;\r\n" + strings.Repeat("x", 131072) + "\r\n\\\\restrict literal\\tdata\r\n\\.\r\n"
	for _, script := range []string{inner, "-- PostgreSQL database dump\r\n\r\n\\restrict originalKey123\r\n" + inner + "\r\n\\unrestrict originalKey123\r\n\r\n"} {
		reader, cleanup, err := prepareBackupRestore(t.Context(), strings.NewReader(script))
		require.NoError(t, err)
		data, err := io.ReadAll(reader)
		cleanup()
		require.NoError(t, err)
		expected := inner
		if strings.HasPrefix(script, "--") {
			expected += "\r\n"
		}
		require.Equal(t, expected, string(data))
	}
}

func TestBackupRestoreScriptRejectsMalformedEnvelope(t *testing.T) {
	for _, script := range []string{
		"\\restrict key\nSELECT 1;\n",
		"\\restrict key\nSELECT 1;\n\\unrestrict another\n",
		"\\restrict key extra\nSELECT 1;\n\\unrestrict key\n",
		"\\restrict bad-key\nSELECT 1;\n\\unrestrict bad-key\n",
	} {
		_, _, err := prepareBackupRestore(t.Context(), strings.NewReader(script))
		require.Error(t, err)
	}
}

type backupRestoreReadFailure struct{}

func (backupRestoreReadFailure) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestBackupRestoreScriptCleansUpOnReadFailureAndCancellation(t *testing.T) {
	temp := t.TempDir()
	t.Setenv("TMPDIR", temp)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	for _, input := range []struct {
		ctx    context.Context
		reader io.Reader
		want   error
	}{
		{t.Context(), io.MultiReader(strings.NewReader("SELECT 1;"), backupRestoreReadFailure{}), io.ErrUnexpectedEOF},
		{ctx, strings.NewReader("SELECT 1;"), context.Canceled},
	} {
		_, _, err := prepareBackupRestore(input.ctx, input.reader)
		require.True(t, errors.Is(err, input.want))
		files, err := os.ReadDir(temp)
		require.NoError(t, err)
		require.Empty(t, files)
	}
}
