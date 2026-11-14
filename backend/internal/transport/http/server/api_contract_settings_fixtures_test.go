//go:build unit

package server_test

import _ "embed"

// Keep the complete administrator settings responses in reviewable fixtures.
// TestAPIContracts compares every field, including defaults for new switches.
//
//go:embed testdata/admin_settings_response.json
var adminSettingsContractJSON string

//go:embed testdata/admin_settings_oauth_defaults_response.json
var adminSettingsOAuthDefaultsContractJSON string
