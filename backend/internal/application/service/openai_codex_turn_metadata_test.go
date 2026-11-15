package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCodexTurnMetadataUsesPrintableASCIIWithoutChangingValues(t *testing.T) {
	for _, raw := range []string{
		`{"label":"中文🚀 café","literal":"\\u4e2d","controls":"\u007f","counter":9007199254740993}`,
		`{"label":"ASCII only","literal":"\\u4e2d"}`,
	} {
		metadata, err := decodeCodexMetadataObject(raw)
		require.NoError(t, err)
		encoded, err := marshalCodexTurnMetadata(metadata)
		require.NoError(t, err)
		decoded, err := decodeCodexMetadataObject(string(encoded))
		require.NoError(t, err)
		require.Equal(t, metadata, decoded)
		require.True(t, json.Valid(encoded))
		for _, b := range encoded {
			require.True(t, b >= 0x20 && b < 0x7f)
		}
	}
	ids := &codexFingerprintIDs{installationID: "gateway-install"}
	rewritten := rewriteCodexTurnMetadataValue(`{"label":"中文🚀"}`, ids)
	for _, b := range []byte(rewritten) {
		require.True(t, b >= 0x20 && b < 0x7f)
	}
}
