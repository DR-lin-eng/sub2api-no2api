package repository

import "github.com/DR-lin-eng/sub2api-no2api/internal/shared/geminicli"

// NewGeminiDriveClient creates a concrete DriveClient for Google Drive API operations.
// Returned as geminicli.DriveClient interface for DI (Strategy A).
func NewGeminiDriveClient() geminicli.DriveClient {
	return geminicli.NewDriveClient()
}
