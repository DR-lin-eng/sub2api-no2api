package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type permissionGroupsSettingRepo struct{ values map[string]string }

func (r *permissionGroupsSettingRepo) Get(ctx context.Context, key string) (*Setting, error) {
	value, err := r.GetValue(ctx, key)
	if err != nil {
		return nil, err
	}
	return &Setting{Key: key, Value: value}, nil
}
func (r *permissionGroupsSettingRepo) GetValue(_ context.Context, key string) (string, error) {
	value, ok := r.values[key]
	if !ok {
		return "", ErrSettingNotFound
	}
	return value, nil
}
func (r *permissionGroupsSettingRepo) Set(_ context.Context, key, value string) error {
	r.values[key] = value
	return nil
}
func (r *permissionGroupsSettingRepo) GetMultiple(context.Context, []string) (map[string]string, error) {
	return nil, nil
}
func (r *permissionGroupsSettingRepo) SetMultiple(context.Context, map[string]string) error {
	return nil
}
func (r *permissionGroupsSettingRepo) GetAll(context.Context) (map[string]string, error) {
	return r.values, nil
}
func (r *permissionGroupsSettingRepo) Delete(context.Context, string) error { return nil }

func TestPermissionGroups_DefaultSupportGroupAndRoleResolution(t *testing.T) {
	repo := &permissionGroupsSettingRepo{values: map[string]string{}}
	svc := NewSettingService(repo, nil)

	groups, err := svc.GetPermissionGroups(context.Background())
	require.NoError(t, err)
	require.Len(t, groups, 1)
	require.Equal(t, "support", groups[0].ID)
	require.Equal(t, "客服", groups[0].Name)

	permissions, err := svc.PermissionsForRole(context.Background(), "support")
	require.NoError(t, err)
	require.ElementsMatch(t, []string{PermissionSupportRead, PermissionSupportWrite, PermissionUsersReadBasic}, permissions)
}

func TestPermissionGroups_RejectsRemovedBuiltInAndUnknownPermission(t *testing.T) {
	_, err := NormalizePermissionGroups([]PermissionGroup{{ID: "ops", Name: "Ops", BuiltIn: false}})
	require.Error(t, err)
	_, err = NormalizePermissionGroups([]PermissionGroup{
		{ID: "support", Name: "客服", Permissions: []string{PermissionSupportRead}, BuiltIn: true},
		{ID: "ops", Name: "Ops", Permissions: []string{"unknown.permission"}},
	})
	require.Error(t, err)
}

func TestPermissionGroups_UpdatePersistsCustomGroup(t *testing.T) {
	repo := &permissionGroupsSettingRepo{values: map[string]string{}}
	svc := NewSettingService(repo, nil)
	groups := append(DefaultPermissionGroups(), PermissionGroup{
		ID: "ops", Name: "运营", Permissions: []string{PermissionDashboardRead},
	})

	updated, err := svc.UpdatePermissionGroups(context.Background(), groups)
	require.NoError(t, err)
	require.Len(t, updated, 2)
	permissions, err := svc.PermissionsForRole(context.Background(), "ops")
	require.NoError(t, err)
	require.Equal(t, []string{PermissionDashboardRead}, permissions)
}
