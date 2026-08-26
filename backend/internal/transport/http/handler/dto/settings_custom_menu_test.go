package dto

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseCustomMenuItemsPreservesExplicitTokenDelegation(t *testing.T) {
	t.Parallel()

	items := ParseCustomMenuItems(`[
		{"id":"trusted","label":"Trusted","url":"https://trusted.example.com","visibility":"user","sort_order":0,"forward_access_token":true,"forward_access_token_in_url":true},
		{"id":"default","label":"Default","url":"https://default.example.com","visibility":"user","sort_order":1}
	]`)

	require.Len(t, items, 2)
	require.True(t, items[0].ForwardAccessToken)
	require.True(t, items[0].ForwardAccessTokenInURL)
	require.False(t, items[1].ForwardAccessToken)
	require.False(t, items[1].ForwardAccessTokenInURL)
}

func TestParseUserVisibleMenuItemsKeepsDelegationFlagAndFiltersAdminItems(t *testing.T) {
	t.Parallel()

	items := ParseUserVisibleMenuItems(`[
		{"id":"user","label":"User","url":"https://user.example.com","visibility":"user","sort_order":0,"forward_access_token":true},
		{"id":"admin","label":"Admin","url":"https://admin.example.com","visibility":"admin","sort_order":1,"forward_access_token":true}
	]`)

	require.Len(t, items, 1)
	require.Equal(t, "user", items[0].ID)
	require.True(t, items[0].ForwardAccessToken)
}

func TestParseUserVisibleMenuItemsPreservesURLTokenFlag(t *testing.T) {
	t.Parallel()

	items := ParseUserVisibleMenuItems(`[
		{"id":"user","label":"User","url":"https://user.example.com","visibility":"user","sort_order":0,"forward_access_token_in_url":true},
		{"id":"admin","label":"Admin","url":"https://admin.example.com","visibility":"admin","sort_order":1,"forward_access_token_in_url":true}
	]`)

	require.Len(t, items, 1)
	require.True(t, items[0].ForwardAccessTokenInURL)
}
