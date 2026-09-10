package service

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestRewriteUserIDWithSessionID_InsertsAndReplacesMetadata(t *testing.T) {
	svc := NewIdentityService(nil)
	account := &Account{ID: 990001, Platform: PlatformAnthropic, Type: AccountTypeOAuth}
	sessionID := "11111111-2222-4333-8444-555555555555"

	inserted, err := svc.RewriteUserIDWithSessionID(
		[]byte(`{"messages":[],"metadata":{}}`),
		account,
		"account-uuid",
		"client-id",
		"claude-cli/2.1.78 (external, cli)",
		sessionID,
	)
	require.NoError(t, err)
	insertedParsed := ParseMetadataUserID(gjson.GetBytes(inserted, "metadata.user_id").String())
	require.NotNil(t, insertedParsed)
	require.Equal(t, sessionID, insertedParsed.SessionID)
	require.Equal(t, "client-id", insertedParsed.DeviceID)

	original := FormatMetadataUserID("client-id", "account-uuid", "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "2.1.78")
	replaced, err := svc.RewriteUserIDWithSessionID(
		[]byte(`{"metadata":{"user_id":`+strconv.Quote(original)+`}}`),
		account,
		"account-uuid",
		"client-id",
		"claude-cli/2.1.78 (external, cli)",
		sessionID,
	)
	require.NoError(t, err)
	replacedParsed := ParseMetadataUserID(gjson.GetBytes(replaced, "metadata.user_id").String())
	require.NotNil(t, replacedParsed)
	require.Equal(t, sessionID, replacedParsed.SessionID)
}
