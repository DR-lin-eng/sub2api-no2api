//go:build unit

package service

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type updateServiceCacheStub struct {
	data string
}

func (s *updateServiceCacheStub) GetUpdateInfo(context.Context) (string, error) {
	if s.data == "" {
		return "", errors.New("cache miss")
	}
	return s.data, nil
}

func (s *updateServiceCacheStub) SetUpdateInfo(_ context.Context, data string, _ time.Duration) error {
	s.data = data
	return nil
}

type updateServiceGitHubClientStub struct {
	release        *GitHubRelease
	latestErr      error
	recentReleases []*GitHubRelease
	recentErr      error
	latestRepo     string
	recentRepo     string
}

func (s *updateServiceGitHubClientStub) FetchLatestRelease(_ context.Context, repo string) (*GitHubRelease, error) {
	s.latestRepo = repo
	return s.release, s.latestErr
}

func (s *updateServiceGitHubClientStub) FetchRecentReleases(_ context.Context, repo string, _ int) ([]*GitHubRelease, error) {
	s.recentRepo = repo
	return s.recentReleases, s.recentErr
}

func (s *updateServiceGitHubClientStub) DownloadFile(context.Context, string, string, int64) error {
	panic("DownloadFile should not be called when no update is available")
}

func (s *updateServiceGitHubClientStub) FetchReleaseFile(context.Context, string, int64) ([]byte, error) {
	panic("FetchReleaseFile should not be called when no update is available")
}

func TestUpdateServicePerformUpdateNoUpdateReturnsSentinel(t *testing.T) {
	client := &updateServiceGitHubClientStub{
		release: &GitHubRelease{
			TagName: "v0.1.132",
			Name:    "v0.1.132",
		},
	}
	svc := NewUpdateService(
		&updateServiceCacheStub{},
		client,
		"0.1.132",
		"release",
	)

	err := svc.PerformUpdate(context.Background())

	require.Error(t, err)
	require.True(t, errors.Is(err, ErrNoUpdateAvailable))
	require.ErrorIs(t, err, ErrNoUpdateAvailable)
	require.Equal(t, "DR-lin-eng/sub2api-no2api", client.latestRepo)
}

func TestUpdateServicePerformUpdateFailsWhenLiveReleaseLookupFails(t *testing.T) {
	latestErr := errors.New("github unavailable")
	client := &updateServiceGitHubClientStub{latestErr: latestErr}
	svc := NewUpdateService(
		&updateServiceCacheStub{},
		client,
		"0.1.132",
		"release",
	)

	err := svc.PerformUpdate(context.Background())

	require.ErrorIs(t, err, latestErr)
	require.NotErrorIs(t, err, ErrNoUpdateAvailable)
	require.Equal(t, "DR-lin-eng/sub2api-no2api", client.latestRepo)
}

func TestUpdateServiceResolveReleaseVersionFreezesStableTarget(t *testing.T) {
	client := &updateServiceGitHubClientStub{
		recentReleases: []*GitHubRelease{
			{TagName: "v1.2.4-beta.1", Prerelease: true},
			{TagName: "v1.2.4", Name: "v1.2.4"},
		},
	}
	svc := NewUpdateService(&updateServiceCacheStub{}, client, "1.2.3", "release")

	version, err := svc.ResolveReleaseVersion(context.Background(), "v1.2.4")

	require.NoError(t, err)
	require.Equal(t, "1.2.4", version)
	require.Equal(t, githubRepo, client.recentRepo)
}

func TestUpdateServiceResolveReleaseVersionRejectsUnknownTarget(t *testing.T) {
	client := &updateServiceGitHubClientStub{recentReleases: []*GitHubRelease{{TagName: "v1.2.3"}}}
	svc := NewUpdateService(&updateServiceCacheStub{}, client, "1.2.3", "release")

	_, err := svc.ResolveReleaseVersion(context.Background(), "1.2.99")

	require.ErrorIs(t, err, ErrReleaseVersionNotFound)
}

func TestApplyReleaseAssetsRequiresSignedChecksumManifest(t *testing.T) {
	svc := NewUpdateService(&updateServiceCacheStub{}, &updateServiceGitHubClientStub{}, "0.1.132", "release")
	archive := Asset{
		Name:        "sub2api_0.1.133_" + svc.getArchiveName() + ".tar.gz",
		DownloadURL: "https://github.com/DR-lin-eng/sub2api-no2api/releases/download/v0.1.133/sub2api.tar.gz",
	}

	err := svc.applyReleaseAssets(context.Background(), []Asset{archive})
	require.ErrorContains(t, err, "missing checksums.txt")

	err = svc.applyReleaseAssets(context.Background(), []Asset{
		archive,
		{Name: "checksums.txt", DownloadURL: "https://github.com/DR-lin-eng/sub2api-no2api/releases/download/v0.1.133/checksums.txt"},
	})
	require.ErrorContains(t, err, "missing checksums.txt.sig")
}

func TestVerifySignedChecksumManifest(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	publicKeyBase64 := base64.StdEncoding.EncodeToString(publicKey)
	manifest := []byte("abc123  sub2api_linux_amd64.tar.gz\n")
	signature := ed25519.Sign(privateKey, manifest)

	require.NoError(t, verifySignedChecksumManifestWithPublicKey(manifest, signature, publicKeyBase64))
	signature[0] ^= 0xff
	require.Error(t, verifySignedChecksumManifestWithPublicKey(manifest, signature, publicKeyBase64))
}

func TestVerifyChecksumData(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "sub2api_linux_amd64.tar.gz")
	require.NoError(t, os.WriteFile(filePath, []byte("payload"), 0o600))

	manifest := []byte("239f59ed55e737c77147cf55ad7c5f9b7f9e4f4f3db2a4d9bbd3a3a4adf3ad5d  sub2api_linux_amd64.tar.gz\n")
	require.Error(t, verifyChecksumData(filePath, manifest))
}

func TestUpdateServiceCheckUpdateComparesReleaseCoreWhenCurrentVersionHasBuildSuffix(t *testing.T) {
	tests := []struct {
		name      string
		current   string
		latest    string
		hasUpdate bool
	}{
		{
			name:      "patched build matches its upstream release",
			current:   "0.1.156-failover-r3",
			latest:    "v0.1.156",
			hasUpdate: false,
		},
		{
			name:      "build metadata matches its upstream release",
			current:   "0.1.156+failover.r3",
			latest:    "v0.1.156",
			hasUpdate: false,
		},
		{
			name:      "patched build still detects a newer upstream release",
			current:   "0.1.156-failover-r3",
			latest:    "v0.1.157",
			hasUpdate: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewUpdateService(
				&updateServiceCacheStub{},
				&updateServiceGitHubClientStub{
					release: &GitHubRelease{TagName: tt.latest, Name: tt.latest},
				},
				tt.current,
				"release",
			)

			info, err := svc.CheckUpdate(context.Background(), true)

			require.NoError(t, err)
			require.Equal(t, tt.current, info.CurrentVersion)
			require.Equal(t, tt.hasUpdate, info.HasUpdate)
		})
	}
}

func newRollbackTestService(current string, releases []*GitHubRelease) *UpdateService {
	return NewUpdateService(
		&updateServiceCacheStub{},
		&updateServiceGitHubClientStub{recentReleases: releases},
		current,
		"release",
	)
}

func TestUpdateServiceListRollbackVersionsFiltersAndCaps(t *testing.T) {
	releases := []*GitHubRelease{
		{TagName: "v0.1.148", PublishedAt: "2026-07-09T00:00:00Z"},                       // newer than current: excluded
		{TagName: "v0.1.147", PublishedAt: "2026-07-08T00:00:00Z"},                       // current: excluded
		{TagName: "v0.1.146-rc1", PublishedAt: "2026-07-07T12:00:00Z", Prerelease: true}, // prerelease: excluded
		{TagName: "v0.1.146", PublishedAt: "2026-07-07T00:00:00Z"},
		{TagName: "v0.1.145", PublishedAt: "2026-07-06T00:00:00Z", Draft: true}, // draft: excluded
		{TagName: "v0.1.144", PublishedAt: "2026-07-05T00:00:00Z"},
		{TagName: "v0.1.144", PublishedAt: "2026-07-05T00:00:00Z"}, // duplicate: excluded
		{TagName: "v0.1.143", PublishedAt: "2026-07-04T00:00:00Z"},
		{TagName: "v0.1.142", PublishedAt: "2026-07-03T00:00:00Z"}, // beyond cap of 3: excluded
	}
	svc := newRollbackTestService("0.1.147", releases)

	versions, err := svc.ListRollbackVersions(context.Background())

	require.NoError(t, err)
	require.Len(t, versions, 3)
	require.Equal(t, "0.1.146", versions[0].Version)
	require.Equal(t, "0.1.144", versions[1].Version)
	require.Equal(t, "0.1.143", versions[2].Version)
}

func TestUpdateServiceListRollbackVersionsSortsUnorderedInput(t *testing.T) {
	releases := []*GitHubRelease{
		{TagName: "v0.1.144"},
		{TagName: "v0.1.146"},
		{TagName: "v0.1.145"},
	}
	svc := newRollbackTestService("0.1.147", releases)

	versions, err := svc.ListRollbackVersions(context.Background())

	require.NoError(t, err)
	require.Len(t, versions, 3)
	require.Equal(t, "0.1.146", versions[0].Version)
	require.Equal(t, "0.1.145", versions[1].Version)
	require.Equal(t, "0.1.144", versions[2].Version)
}

func TestUpdateServiceListRollbackVersionsEmptyWhenNoneOlder(t *testing.T) {
	releases := []*GitHubRelease{
		{TagName: "v0.1.147"},
		{TagName: "v0.1.148"},
	}
	svc := newRollbackTestService("0.1.147", releases)

	versions, err := svc.ListRollbackVersions(context.Background())

	require.NoError(t, err)
	require.Empty(t, versions)
}

func TestUpdateServiceListRollbackVersionsPropagatesFetchError(t *testing.T) {
	svc := NewUpdateService(
		&updateServiceCacheStub{},
		&updateServiceGitHubClientStub{recentErr: errors.New("github unavailable")},
		"0.1.147",
		"release",
	)

	_, err := svc.ListRollbackVersions(context.Background())

	require.Error(t, err)
	require.Contains(t, err.Error(), "github unavailable")
}

func TestUpdateServiceRollbackToVersionRejectsDisallowedTargets(t *testing.T) {
	releases := []*GitHubRelease{
		{TagName: "v0.1.148"},
		{TagName: "v0.1.147"},
		{TagName: "v0.1.146"},
		{TagName: "v0.1.145"},
		{TagName: "v0.1.144"},
		{TagName: "v0.1.143"},
		{TagName: "v0.1.142"},
	}
	svc := newRollbackTestService("0.1.147", releases)

	for _, target := range []string{
		"",         // empty
		"0.1.147",  // current version
		"v0.1.147", // current version with prefix
		"0.1.148",  // newer than current
		"0.1.142",  // older than the 3 most recent
		"9.9.9",    // nonexistent
	} {
		err := svc.RollbackToVersion(context.Background(), target)
		require.ErrorIs(t, err, ErrRollbackVersionNotAllowed, "target %q should be rejected", target)
	}
}

func TestUpdateServiceRollbackToVersionAcceptsVPrefix(t *testing.T) {
	// No platform asset in the release: the target passes the allowlist check
	// and fails later at asset lookup, proving the version itself was accepted.
	releases := []*GitHubRelease{
		{TagName: "v0.1.147"},
		{TagName: "v0.1.146"},
	}
	svc := newRollbackTestService("0.1.147", releases)

	err := svc.RollbackToVersion(context.Background(), "v0.1.146")

	require.Error(t, err)
	require.NotErrorIs(t, err, ErrRollbackVersionNotAllowed)
	require.Contains(t, err.Error(), "no compatible release found")
}
