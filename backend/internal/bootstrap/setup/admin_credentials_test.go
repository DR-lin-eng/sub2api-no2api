package setup

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/gin-gonic/gin/binding"
)

func TestPrepareAdminCredentialsGeneratesMissingValues(t *testing.T) {
	t.Parallel()

	admin := AdminConfig{Email: "  ", Password: ""}
	emailGenerated, passwordGenerated, err := prepareAdminCredentials(&admin)
	if err != nil {
		t.Fatalf("prepareAdminCredentials() error = %v", err)
	}
	if !emailGenerated || !passwordGenerated {
		t.Fatalf("generated flags = (%v, %v), want (true, true)", emailGenerated, passwordGenerated)
	}
	if !regexp.MustCompile(`^admin-[0-9a-f]{12}@sub2api\.local$`).MatchString(admin.Email) {
		t.Fatalf("generated email = %q, want admin-<12 hex>@sub2api.local", admin.Email)
	}
	// 生成的邮箱必须能通过登录接口的 binding:"required,email" 校验。
	loginReq := struct {
		Email string `binding:"required,email"`
	}{Email: admin.Email}
	if err := binding.Validator.ValidateStruct(&loginReq); err != nil {
		t.Fatalf("generated email %q rejected by login validator: %v", admin.Email, err)
	}
	if len(admin.Password) != 32 {
		t.Fatalf("generated password length = %d, want 32", len(admin.Password))
	}

	other := AdminConfig{}
	if _, _, err := prepareAdminCredentials(&other); err != nil {
		t.Fatalf("prepareAdminCredentials() second call error = %v", err)
	}
	if other.Email == admin.Email {
		t.Fatalf("generated emails should be random, got %q twice", admin.Email)
	}
}

func TestPrepareAdminCredentialsKeepsProvidedValues(t *testing.T) {
	t.Parallel()

	admin := AdminConfig{Email: "owner@example.com", Password: "a-strong-password"}
	emailGenerated, passwordGenerated, err := prepareAdminCredentials(&admin)
	if err != nil {
		t.Fatalf("prepareAdminCredentials() error = %v", err)
	}
	if emailGenerated || passwordGenerated {
		t.Fatalf("generated flags = (%v, %v), want (false, false)", emailGenerated, passwordGenerated)
	}
	if admin.Email != "owner@example.com" || admin.Password != "a-strong-password" {
		t.Fatalf("provided credentials were modified: %+v", admin)
	}
}

func TestPrepareAdminCredentialsRejectsWeakPassword(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		password string
	}{
		{name: "too short", password: "123456"},
		{name: "seven chars", password: "1234567"},
		{name: "exceeds bcrypt limit", password: strings.Repeat("a", 73)},
		{name: "former 128 limit", password: strings.Repeat("a", 128)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			admin := AdminConfig{Email: "owner@example.com", Password: tt.password}
			_, _, err := prepareAdminCredentials(&admin)
			if err == nil || !strings.Contains(err.Error(), "invalid admin password") {
				t.Fatalf("prepareAdminCredentials(%q) error = %v, want invalid admin password error", tt.password, err)
			}
		})
	}
}

func TestPrepareAdminCredentialsRejectsUnloginableEmail(t *testing.T) {
	t.Parallel()

	for _, email := range []string{"admin", "a@b", "Owner <owner@example.com>", "<owner@example.com>"} {
		t.Run(email, func(t *testing.T) {
			t.Parallel()

			admin := AdminConfig{Email: email, Password: "a-strong-password"}
			_, _, err := prepareAdminCredentials(&admin)
			if err == nil || !strings.Contains(err.Error(), "invalid admin email") {
				t.Fatalf("prepareAdminCredentials(%q) error = %v, want invalid admin email error", email, err)
			}
		})
	}
}

func TestPrepareAdminCredentialsTrimsProvidedEmail(t *testing.T) {
	t.Parallel()

	admin := AdminConfig{Email: "  owner@example.com\n", Password: "a-strong-password"}
	if _, _, err := prepareAdminCredentials(&admin); err != nil {
		t.Fatalf("prepareAdminCredentials() error = %v", err)
	}
	if admin.Email != "owner@example.com" {
		t.Fatalf("email = %q, want trimmed owner@example.com", admin.Email)
	}
}

func TestPrepareAdminCredentialsAcceptsBcryptMaxLengthPassword(t *testing.T) {
	t.Parallel()

	admin := AdminConfig{Email: "owner@example.com", Password: strings.Repeat("a", 72)}
	if _, _, err := prepareAdminCredentials(&admin); err != nil {
		t.Fatalf("prepareAdminCredentials() error = %v", err)
	}
	// 校验上限必须与 bcrypt 实际可哈希的上限一致，否则通过校验后仍会在建号时失败。
	user := service.User{}
	if err := user.SetPassword(admin.Password); err != nil {
		t.Fatalf("SetPassword() with max-length password error = %v", err)
	}
}

func expectAdminBootstrapCounts(mock sqlmock.Sqlmock, totalUsers, adminUsers int64) {
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(1) FROM users")).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(totalUsers))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(1) FROM users WHERE role = $1")).
		WithArgs(service.RoleAdmin).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(adminUsers))
}

func TestBootstrapAdminUserSkipsValidationWhenNotCreating(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		totalUsers int64
		adminUsers int64
		reason     string
	}{
		{name: "admin exists", totalUsers: 3, adminUsers: 1, reason: adminBootstrapReasonAdminExists},
		{name: "users exist without admin", totalUsers: 3, adminUsers: 0, reason: adminBootstrapReasonUsersExistWithoutAdmin},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock.New() error = %v", err)
			}
			defer func() { _ = db.Close() }()
			expectAdminBootstrapCounts(mock, tt.totalUsers, tt.adminUsers)

			// 已有部署里遗留的弱密码/非法邮箱不能阻断启动。
			cfg := &SetupConfig{Admin: AdminConfig{Email: "admin", Password: "123456"}}
			created, reason, err := bootstrapAdminUser(context.Background(), db, cfg)
			if err != nil || created || reason != tt.reason {
				t.Fatalf("bootstrapAdminUser() = (%v, %q, %v), want (false, %q, nil)", created, reason, err, tt.reason)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unexpected database interaction: %v", err)
			}
		})
	}
}

func TestBootstrapAdminUserRejectsWeakPasswordWithoutInsert(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer func() { _ = db.Close() }()
	expectAdminBootstrapCounts(mock, 0, 0)

	cfg := &SetupConfig{Admin: AdminConfig{Password: "123456"}}
	created, _, err := bootstrapAdminUser(context.Background(), db, cfg)
	if err == nil || created || !strings.Contains(err.Error(), "invalid admin password") {
		t.Fatalf("bootstrapAdminUser() = (%v, %v), want invalid admin password error", created, err)
	}
	// 未设置 INSERT 期望：若发生插入，sqlmock 会返回非预期调用错误，上面的错误断言即失败。
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unexpected database interaction: %v", err)
	}
}

func TestBootstrapAdminUserCreatesAdminWithGeneratedCredentials(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer func() { _ = db.Close() }()
	expectAdminBootstrapCounts(mock, 0, 0)
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO users")).
		WithArgs(
			sqlmock.AnyArg(), sqlmock.AnyArg(), service.RoleAdmin, sqlmock.AnyArg(),
			sqlmock.AnyArg(), service.StatusActive, sqlmock.AnyArg(), sqlmock.AnyArg(),
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	cfg := &SetupConfig{}
	created, reason, err := bootstrapAdminUser(context.Background(), db, cfg)
	if err != nil || !created || reason != adminBootstrapReasonEmptyDatabase {
		t.Fatalf("bootstrapAdminUser() = (%v, %q, %v), want (true, %q, nil)", created, reason, err, adminBootstrapReasonEmptyDatabase)
	}
	if !regexp.MustCompile(`^admin-[0-9a-f]{12}@sub2api\.local$`).MatchString(cfg.Admin.Email) {
		t.Fatalf("admin email = %q, want generated admin-<12 hex>@sub2api.local", cfg.Admin.Email)
	}
	if len(cfg.Admin.Password) != 32 {
		t.Fatalf("admin password length = %d, want generated 32", len(cfg.Admin.Password))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations not met: %v", err)
	}
}

func TestPrepareAdminCredentialsAcceptsMinimumLengthPassword(t *testing.T) {
	t.Parallel()

	admin := AdminConfig{Password: "12345678"}
	emailGenerated, passwordGenerated, err := prepareAdminCredentials(&admin)
	if err != nil {
		t.Fatalf("prepareAdminCredentials() error = %v", err)
	}
	if !emailGenerated || passwordGenerated {
		t.Fatalf("generated flags = (%v, %v), want (true, false)", emailGenerated, passwordGenerated)
	}
	if admin.Password != "12345678" {
		t.Fatalf("password = %q, want unchanged", admin.Password)
	}
}
