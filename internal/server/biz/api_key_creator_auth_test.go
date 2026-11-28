// 职责: 验证 API Key 鉴权跟随创建人的账号状态与项目成员身份，移出项目会停用其个人 Key。
// 关联文件:
//   internal/server/biz/auth.go
//   internal/server/biz/user.go
//   internal/server/biz/api_key.go
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
	"github.com/looplj/axonhub/internal/ent/project"
	"github.com/looplj/axonhub/internal/ent/user"
	"github.com/looplj/axonhub/internal/pkg/xcache"
)

type creatorAuthFixture struct {
	auth     *AuthService
	client   *ent.Client
	ctx      context.Context
	project  *ent.Project
	employee *ent.User
	shared   *ent.APIKey
	personal *ent.APIKey
}

func setupCreatorAuthFixture(t *testing.T) *creatorAuthFixture {
	t.Helper()

	authService, client, cleanup := setupTestAuthService(t, xcache.Config{Mode: xcache.ModeMemory})
	t.Cleanup(func() {
		cleanup()
		_ = client.Close()
	})

	ctx := authz.WithTestBypass(ent.NewContext(context.Background(), client))
	ctx = contexts.WithUser(ctx, createOwnerUser(t, ctx, client))

	proj, err := client.Project.Create().SetName("customer").SetStatus(project.StatusActive).Save(ctx)
	require.NoError(t, err)

	employee, err := client.User.Create().SetEmail("employee@customer.test").SetPassword("x").
		SetStatus(user.StatusActivated).Save(ctx)
	require.NoError(t, err)
	_, err = client.UserProject.Create().SetUserID(employee.ID).SetProjectID(proj.ID).Save(ctx)
	require.NoError(t, err)

	newKey := func(key string, keyType apikey.Type) *ent.APIKey {
		k, err := client.APIKey.Create().SetKey(key).SetName(key).SetType(keyType).
			SetUser(employee).SetProject(proj).Save(ctx)
		require.NoError(t, err)

		return k
	}

	return &creatorAuthFixture{
		auth:     authService,
		client:   client,
		ctx:      ctx,
		project:  proj,
		employee: employee,
		shared:   newKey("ah-creator-shared", apikey.TypeUser),
		personal: newKey("ah-creator-personal", apikey.TypePersonal),
	}
}

func TestAuthenticateAPIKey_ActiveMemberCreatorIsAccepted(t *testing.T) {
	f := setupCreatorAuthFixture(t)

	_, err := f.auth.AuthenticateAPIKey(f.ctx, f.shared.Key)
	require.NoError(t, err)
	_, err = f.auth.AuthenticateAPIKey(f.ctx, f.personal.Key)
	require.NoError(t, err)
}

func TestAuthenticateAPIKey_RejectsKeysOfDeactivatedCreator(t *testing.T) {
	f := setupCreatorAuthFixture(t)

	// Warm the user cache so the test also covers its invalidation.
	_, err := f.auth.AuthenticateAPIKey(f.ctx, f.shared.Key)
	require.NoError(t, err)

	_, err = f.auth.UserService.UpdateUserStatus(f.ctx, f.employee.ID, user.StatusDeactivated)
	require.NoError(t, err)

	_, err = f.auth.AuthenticateAPIKey(f.ctx, f.shared.Key)
	require.ErrorIs(t, err, ErrInvalidAPIKey)
	_, err = f.auth.AuthenticateAPIKey(f.ctx, f.personal.Key)
	require.ErrorIs(t, err, ErrInvalidAPIKey)
}

func TestAuthenticateAPIKey_RejectsKeysAfterCreatorLeavesProject(t *testing.T) {
	f := setupCreatorAuthFixture(t)

	_, err := f.auth.AuthenticateAPIKey(f.ctx, f.shared.Key)
	require.NoError(t, err)

	require.NoError(t, f.auth.UserService.RemoveUserFromProject(f.ctx, f.employee.ID, f.project.ID))

	_, err = f.auth.AuthenticateAPIKey(f.ctx, f.shared.Key)
	require.ErrorIs(t, err, ErrInvalidAPIKey)

	personal, err := f.client.APIKey.Get(f.ctx, f.personal.ID)
	require.NoError(t, err)
	require.Equal(t, apikey.StatusDisabled, personal.Status)

	shared, err := f.client.APIKey.Get(f.ctx, f.shared.ID)
	require.NoError(t, err)
	require.Equal(t, apikey.StatusEnabled, shared.Status, "shared keys are blocked at authentication, not disabled")
}

func TestAuthenticateAPIKey_SystemOwnerCreatorNeedsNoMembership(t *testing.T) {
	f := setupCreatorAuthFixture(t)

	owner, ok := contexts.GetUser(f.ctx)
	require.True(t, ok)

	key, err := f.client.APIKey.Create().SetKey("ah-creator-owner").SetName("owner").
		SetUserID(owner.ID).SetProject(f.project).Save(f.ctx)
	require.NoError(t, err)

	_, err = f.auth.AuthenticateAPIKey(f.ctx, key.Key)
	require.NoError(t, err)
}

func TestRemoveUserFromProject_DisablesPersonalKeysOnlyInThatProject(t *testing.T) {
	f := setupCreatorAuthFixture(t)

	otherProject, err := f.client.Project.Create().SetName("other").SetStatus(project.StatusActive).Save(f.ctx)
	require.NoError(t, err)
	_, err = f.client.UserProject.Create().SetUserID(f.employee.ID).SetProjectID(otherProject.ID).Save(f.ctx)
	require.NoError(t, err)
	otherKey, err := f.client.APIKey.Create().SetKey("ah-creator-other").SetName("other").
		SetType(apikey.TypePersonal).SetUser(f.employee).SetProject(otherProject).Save(f.ctx)
	require.NoError(t, err)

	require.NoError(t, f.auth.UserService.RemoveUserFromProject(f.ctx, f.employee.ID, f.project.ID))

	otherKey, err = f.client.APIKey.Get(f.ctx, otherKey.ID)
	require.NoError(t, err)
	require.Equal(t, apikey.StatusEnabled, otherKey.Status)

	_, err = f.auth.AuthenticateAPIKey(f.ctx, otherKey.Key)
	require.NoError(t, err)
}
