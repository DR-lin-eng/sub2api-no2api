//go:build unit

package service

import (
	"crypto/sha256"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPlanBackupFilePartsUsesRangesFromSingleArchive(t *testing.T) {
	tempDir := t.TempDir()
	archive, err := os.CreateTemp(tempDir, "backup-range-plan-*")
	require.NoError(t, err)
	path := archive.Name()
	t.Cleanup(func() { _ = cleanupBackupFiles(path) })
	content := []byte("0123456789")
	_, err = archive.Write(content)
	require.NoError(t, err)
	require.NoError(t, archive.Close())

	plans, err := planBackupFileParts(path, 4, func(index int) string {
		return fmt.Sprintf("backup/part-%d", index)
	})
	require.NoError(t, err)
	require.Len(t, plans, 3)
	require.Equal(t, []int64{0, 4, 8}, []int64{plans[0].Offset, plans[1].Offset, plans[2].Offset})
	require.Equal(t, []int64{4, 4, 2}, []int64{plans[0].Part.SizeBytes, plans[1].Part.SizeBytes, plans[2].Part.SizeBytes})

	for i, plan := range plans {
		chunk := content[plan.Offset : plan.Offset+plan.Part.SizeBytes]
		require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(chunk)), plan.Part.SHA256)
		require.Equal(t, fmt.Sprintf("backup/part-%d", i+1), plan.Part.S3Key)
	}

	entries, err := os.ReadDir(tempDir)
	require.NoError(t, err)
	require.Len(t, entries, 1, "range planning must not create per-part files")
}

func TestPlanBackupFilePartsRejectsInvalidInputs(t *testing.T) {
	_, err := planBackupFileParts("missing", 0, func(int) string { return "part" })
	require.ErrorContains(t, err, "positive")
}
