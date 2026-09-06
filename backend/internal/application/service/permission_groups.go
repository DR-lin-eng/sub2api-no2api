package service

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

const PermissionGroupsSettingKey = "permission_groups"

const (
	PermissionSupportRead      = "support.read"
	PermissionSupportWrite     = "support.write"
	PermissionSupportTransfer  = "support.balance_transfer"
	PermissionUsersReadBasic   = "users.read_basic"
	PermissionUsersManage      = "users.manage"
	PermissionUsersCredentials = "users.credentials"
	PermissionUsersBilling     = "users.billing"
	PermissionDashboardRead    = "dashboard.read"
	PermissionSettingsManage   = "settings.manage"
	PermissionGroupsManage     = "groups.manage"
	PermissionAccountsManage   = "accounts.manage"
)

type PermissionDefinition struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type PermissionGroup struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Permissions []string `json:"permissions"`
	BuiltIn     bool     `json:"built_in"`
}

var permissionGroupIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,19}$`)

func PermissionDefinitions() []PermissionDefinition {
	return []PermissionDefinition{
		{Key: PermissionSupportRead, Name: "在线客服：查看", Description: "查看客服会话、消息和未读计数"},
		{Key: PermissionSupportWrite, Name: "在线客服：处理", Description: "回复、标记、撤回和处理客服会话"},
		{Key: PermissionSupportTransfer, Name: "在线客服：余额转账", Description: "向用户执行客服余额转账"},
		{Key: PermissionUsersReadBasic, Name: "用户：查看基本信息", Description: "通过客服会话查看基本资料，不含密钥和登录身份"},
		{Key: PermissionUsersManage, Name: "用户：管理", Description: "查看用户列表并创建、编辑、删除用户"},
		{Key: PermissionUsersCredentials, Name: "用户：身份和密钥", Description: "查看和管理用户 API Key、登录身份绑定"},
		{Key: PermissionUsersBilling, Name: "用户：余额和配额", Description: "查看和修改用户余额、配额和 RPM 状态"},
		{Key: PermissionDashboardRead, Name: "仪表盘：查看", Description: "查看管理仪表盘和基础运营概览"},
		{Key: PermissionSettingsManage, Name: "系统设置：管理", Description: "修改系统设置；权限组和管理员密钥仅限完整管理员"},
		{Key: PermissionGroupsManage, Name: "分组：管理", Description: "管理模型分组和分组路由"},
		{Key: PermissionAccountsManage, Name: "账号：管理", Description: "管理上游账号、代理和出口配置"},
	}
}

func permissionDefinitionKeys() map[string]struct{} {
	out := make(map[string]struct{}, len(PermissionDefinitions()))
	for _, item := range PermissionDefinitions() {
		out[item.Key] = struct{}{}
	}
	return out
}

func DefaultPermissionGroups() []PermissionGroup {
	return []PermissionGroup{{
		ID:          "support",
		Name:        "客服",
		Permissions: []string{PermissionSupportRead, PermissionSupportWrite, PermissionUsersReadBasic},
		BuiltIn:     true,
	}}
}

func NormalizePermissionGroups(groups []PermissionGroup) ([]PermissionGroup, error) {
	if len(groups) == 0 {
		return nil, fmt.Errorf("at least one permission group is required")
	}
	knownPermissions := permissionDefinitionKeys()
	seenIDs := make(map[string]struct{}, len(groups))
	seenNames := make(map[string]struct{}, len(groups))
	seenBuiltIns := make(map[string]struct{})
	out := make([]PermissionGroup, 0, len(groups))
	for _, input := range groups {
		item := PermissionGroup{
			ID:          strings.TrimSpace(input.ID),
			Name:        strings.TrimSpace(input.Name),
			Permissions: append([]string(nil), input.Permissions...),
			BuiltIn:     strings.TrimSpace(input.ID) == "support",
		}
		if !permissionGroupIDPattern.MatchString(item.ID) || item.ID == RoleAdmin || item.ID == RoleUser {
			return nil, fmt.Errorf("invalid permission group id %q", item.ID)
		}
		if item.Name == "" || len([]rune(item.Name)) > 50 {
			return nil, fmt.Errorf("permission group %q must have a name of 1-50 characters", item.ID)
		}
		if _, ok := seenIDs[item.ID]; ok {
			return nil, fmt.Errorf("duplicate permission group id %q", item.ID)
		}
		if _, ok := seenNames[item.Name]; ok {
			return nil, fmt.Errorf("duplicate permission group name %q", item.Name)
		}
		seenIDs[item.ID] = struct{}{}
		seenNames[item.Name] = struct{}{}
		permissions := make(map[string]struct{}, len(item.Permissions))
		for _, permission := range item.Permissions {
			permission = strings.TrimSpace(permission)
			if _, ok := knownPermissions[permission]; !ok {
				return nil, fmt.Errorf("unknown permission %q", permission)
			}
			permissions[permission] = struct{}{}
		}
		item.Permissions = make([]string, 0, len(permissions))
		for permission := range permissions {
			item.Permissions = append(item.Permissions, permission)
		}
		sort.Strings(item.Permissions)
		if len(item.Permissions) == 0 {
			return nil, fmt.Errorf("permission group %q must include at least one permission", item.ID)
		}
		if item.BuiltIn {
			seenBuiltIns[item.ID] = struct{}{}
		}
		out = append(out, item)
	}
	for _, required := range DefaultPermissionGroups() {
		if _, ok := seenBuiltIns[required.ID]; !ok {
			return nil, fmt.Errorf("built-in permission group %q cannot be removed", required.ID)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (s *SettingService) GetPermissionGroups(ctx context.Context) ([]PermissionGroup, error) {
	value, err := s.settingRepo.GetValue(ctx, PermissionGroupsSettingKey)
	if err != nil {
		if err == ErrSettingNotFound {
			return DefaultPermissionGroups(), nil
		}
		return nil, err
	}
	var groups []PermissionGroup
	if err := json.Unmarshal([]byte(value), &groups); err != nil {
		return nil, fmt.Errorf("decode permission groups: %w", err)
	}
	return NormalizePermissionGroups(groups)
}

func (s *SettingService) UpdatePermissionGroups(ctx context.Context, groups []PermissionGroup) ([]PermissionGroup, error) {
	normalized, err := NormalizePermissionGroups(groups)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(normalized)
	if err != nil {
		return nil, fmt.Errorf("encode permission groups: %w", err)
	}
	if err := s.settingRepo.Set(ctx, PermissionGroupsSettingKey, string(payload)); err != nil {
		return nil, err
	}
	if s.onUpdate != nil {
		s.onUpdate()
	}
	return normalized, nil
}

func (s *SettingService) PermissionsForRole(ctx context.Context, role string) ([]string, error) {
	if role == RoleAdmin {
		return []string{"*"}, nil
	}
	if role == RoleUser || strings.TrimSpace(role) == "" {
		return nil, nil
	}
	groups, err := s.GetPermissionGroups(ctx)
	if err != nil {
		return nil, err
	}
	for _, group := range groups {
		if group.ID == role {
			return append([]string(nil), group.Permissions...), nil
		}
	}
	return nil, nil
}

func (s *SettingService) HasPermission(ctx context.Context, role, permission string) (bool, error) {
	permissions, err := s.PermissionsForRole(ctx, role)
	if err != nil {
		return false, err
	}
	for _, item := range permissions {
		if item == "*" || item == permission {
			return true, nil
		}
	}
	return false, nil
}
