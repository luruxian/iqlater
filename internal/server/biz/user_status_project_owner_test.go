// 职责: 验证项目所有者只能停用/恢复只属于自己所拥有项目的员工账号。
// 关联文件:
//   internal/server/biz/user.go
//   internal/server/biz/permission_validator.go
// 遵循的 Rules:
//   .agents/rules/go-general.md

package biz

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/looplj/axonhub/internal/authz"
	"github.com/looplj/axonhub/internal/contexts"
	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/ent/apikey"
	"github.com/looplj/axonhub/internal/ent/role"
	"github.com/looplj/axonhub/internal/ent/user"
)

type ownerStatusFixture struct {
	svc      *UserService
	client   *ent.Client
	setupCtx context.Context
	projectA *ent.Project
	projectB *ent.Project
	ownerA   *ent.User
	devA     *ent.User
}

func setupOwnerStatusFixture(t *testing.T) *ownerStatusFixture {
	t.Helper()

	svc, client := setupTestUserService(t)
	t.Cleanup(func() { _ = client.Close() })
	setupTestUserAPIKeyService(t, svc, client)

	ctx := authz.WithTestBypass(ent.NewContext(context.Background(), client))
	f := &ownerStatusFixture{svc: svc, client: client, setupCtx: ctx}

	var err error
	f.projectA, err = client.Project.Create().SetName("A").Save(ctx)
	require.NoError(t, err)
	f.projectB, err = client.Project.Create().SetName("B").Save(ctx)
	require.NoError(t, err)

	f.ownerA = f.newUser(t, "owner@a.test")
	f.join(t, f.ownerA, f.projectA, true)

	f.devA = f.newUser(t, "dev@a.test")
	f.join(t, f.devA, f.projectA, false)
	devRole, err := client.Role.Create().SetName("Developer").SetLevel(role.LevelProject).
		SetProjectID(f.projectA.ID).SetScopes([]string{"read_api_keys", "write_api_keys", "write_users"}).Save(ctx)
	require.NoError(t, err)
	require.NoError(t, client.User.UpdateOneID(f.devA.ID).AddRoleIDs(devRole.ID).Exec(ctx))

	return f
}

func (f *ownerStatusFixture) newUser(t *testing.T, email string) *ent.User {
	t.Helper()

	u, err := f.client.User.Create().SetEmail(email).SetPassword("x").SetStatus(user.StatusActivated).Save(f.setupCtx)
	require.NoError(t, err)

	return u
}

func (f *ownerStatusFixture) join(t *testing.T, u *ent.User, p *ent.Project, owner bool) {
	t.Helper()

	_, err := f.client.UserProject.Create().SetUserID(u.ID).SetProjectID(p.ID).SetIsOwner(owner).Save(f.setupCtx)
	require.NoError(t, err)
}

// as returns the context the JWT middleware builds for a signed-in user, without any test bypass.
func (f *ownerStatusFixture) as(t *testing.T, u *ent.User) context.Context {
	t.Helper()

	loaded, err := f.client.User.Query().Where(user.IDEQ(u.ID)).WithRoles().WithProjectUsers().Only(f.setupCtx)
	require.NoError(t, err)

	ctx := authz.NewUserContext(ent.NewContext(context.Background(), f.client), u.ID)

	return contexts.WithUser(ctx, loaded)
}

func (f *ownerStatusFixture) status(t *testing.T, u *ent.User) user.Status {
	t.Helper()

	got, err := f.client.User.Get(f.setupCtx, u.ID)
	require.NoError(t, err)

	return got.Status
}

func TestProjectOwnerDeactivatesAndReactivatesOwnEmployee(t *testing.T) {
	f := setupOwnerStatusFixture(t)
	employee := f.newUser(t, "employee@a.test")
	f.join(t, employee, f.projectA, false)

	key, err := f.client.APIKey.Create().SetKey("ah-owner-status-personal").SetName("p").
		SetType(apikey.TypePersonal).SetUser(employee).SetProject(f.projectA).Save(f.setupCtx)
	require.NoError(t, err)

	_, err = f.svc.UpdateUserStatus(f.as(t, f.ownerA), employee.ID, user.StatusDeactivated)
	require.NoError(t, err)
	require.Equal(t, user.StatusDeactivated, f.status(t, employee))

	key, err = f.client.APIKey.Get(f.setupCtx, key.ID)
	require.NoError(t, err)
	require.Equal(t, apikey.StatusDisabled, key.Status)

	_, err = f.svc.UpdateUserStatus(f.as(t, f.ownerA), employee.ID, user.StatusActivated)
	require.NoError(t, err)
	require.Equal(t, user.StatusActivated, f.status(t, employee))
}

func TestProjectOwnerCannotDeactivateEmployeeSharedWithAnotherCustomer(t *testing.T) {
	f := setupOwnerStatusFixture(t)
	shared := f.newUser(t, "shared@ab.test")
	f.join(t, shared, f.projectA, false)
	f.join(t, shared, f.projectB, false)

	_, err := f.svc.UpdateUserStatus(f.as(t, f.ownerA), shared.ID, user.StatusDeactivated)
	require.Error(t, err)
	require.Equal(t, user.StatusActivated, f.status(t, shared))
}

func TestProjectOwnerCannotDeactivateOtherCustomersEmployee(t *testing.T) {
	f := setupOwnerStatusFixture(t)
	employeeB := f.newUser(t, "employee@b.test")
	f.join(t, employeeB, f.projectB, false)

	_, err := f.svc.UpdateUserStatus(f.as(t, f.ownerA), employeeB.ID, user.StatusDeactivated)
	require.Error(t, err)
	require.Equal(t, user.StatusActivated, f.status(t, employeeB))
}

func TestProjectOwnerCannotDeactivateCoOwnerOrSystemUser(t *testing.T) {
	f := setupOwnerStatusFixture(t)

	coOwner := f.newUser(t, "coowner@a.test")
	f.join(t, coOwner, f.projectA, true)
	_, err := f.svc.UpdateUserStatus(f.as(t, f.ownerA), coOwner.ID, user.StatusDeactivated)
	require.Error(t, err)
	require.Equal(t, user.StatusActivated, f.status(t, coOwner))

	systemUser := f.newUser(t, "ops@sys.test")
	require.NoError(t, f.client.User.UpdateOneID(systemUser.ID).SetScopes([]string{"read_channels"}).Exec(f.setupCtx))
	f.join(t, systemUser, f.projectA, false)
	_, err = f.svc.UpdateUserStatus(f.as(t, f.ownerA), systemUser.ID, user.StatusDeactivated)
	require.Error(t, err)
	require.Equal(t, user.StatusActivated, f.status(t, systemUser))
}

func TestNonOwnerMemberCannotDeactivateEmployee(t *testing.T) {
	f := setupOwnerStatusFixture(t)
	employee := f.newUser(t, "employee2@a.test")
	f.join(t, employee, f.projectA, false)

	_, err := f.svc.UpdateUserStatus(f.as(t, f.devA), employee.ID, user.StatusDeactivated)
	require.Error(t, err)
	require.Equal(t, user.StatusActivated, f.status(t, employee))
}
