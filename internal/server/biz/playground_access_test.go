// 职责: 验证 Playground 只允许项目成员在本项目内使用、且只能指定本项目可用的渠道。
// 关联文件:
//   internal/server/biz/playground_access.go
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
	"github.com/looplj/axonhub/internal/ent/channel"
	"github.com/looplj/axonhub/internal/ent/enttest"
	"github.com/looplj/axonhub/internal/ent/role"
	"github.com/looplj/axonhub/internal/ent/user"
	"github.com/looplj/axonhub/internal/objects"
	"github.com/looplj/axonhub/internal/pkg/xerrors"
)

type playgroundFixture struct {
	svc      *ChannelService
	client   *ent.Client
	ctx      context.Context
	projectA *ent.Project
	projectB *ent.Project
	memberA  *ent.User
	viewerA  *ent.User
	sysAdmin *ent.User
	allowed  *ent.Channel
	other    *ent.Channel
	disabled *ent.Channel
}

func setupPlaygroundFixture(t *testing.T) *playgroundFixture {
	t.Helper()

	client := enttest.NewEntClient(t, "sqlite3", "file:playground_access?mode=memory&_fk=1")
	t.Cleanup(func() { _ = client.Close() })

	ctx := authz.WithTestBypass(ent.NewContext(context.Background(), client))
	f := &playgroundFixture{
		svc:    &ChannelService{AbstractService: &AbstractService{db: client}},
		client: client,
		ctx:    ctx,
	}

	newChannel := func(name string, status channel.Status) *ent.Channel {
		ch, err := client.Channel.Create().
			SetType(channel.TypeOpenai).
			SetName(name).
			SetStatus(status).
			SetCredentials(objects.ChannelCredentials{APIKey: "k"}).
			SetSupportedModels([]string{"m"}).
			SetDefaultTestModel("m").
			Save(ctx)
		require.NoError(t, err)

		return ch
	}
	f.allowed = newChannel("allowed", channel.StatusEnabled)
	f.other = newChannel("other", channel.StatusEnabled)
	f.disabled = newChannel("disabled", channel.StatusDisabled)

	var err error
	f.projectA, err = client.Project.Create().SetName("A").
		SetProfiles(&objects.ProjectProfiles{
			ActiveProfile: "default",
			Profiles:      []objects.ProjectProfile{{Name: "default", ChannelIDs: []int{f.allowed.ID, f.disabled.ID}}},
		}).Save(ctx)
	require.NoError(t, err)
	f.projectB, err = client.Project.Create().SetName("B").Save(ctx)
	require.NoError(t, err)

	newUser := func(email string) *ent.User {
		u, err := client.User.Create().SetEmail(email).SetPassword("x").SetStatus(user.StatusActivated).Save(ctx)
		require.NoError(t, err)

		return u
	}

	f.memberA = newUser("member@a.test")
	devRole, err := client.Role.Create().SetName("Developer").SetLevel(role.LevelProject).
		SetProjectID(f.projectA.ID).SetScopes([]string{"read_requests", "write_requests"}).Save(ctx)
	require.NoError(t, err)
	_, err = client.UserProject.Create().SetUserID(f.memberA.ID).SetProjectID(f.projectA.ID).Save(ctx)
	require.NoError(t, err)
	require.NoError(t, client.User.UpdateOneID(f.memberA.ID).AddRoleIDs(devRole.ID).Exec(ctx))

	f.viewerA = newUser("viewer@a.test")
	_, err = client.UserProject.Create().SetUserID(f.viewerA.ID).SetProjectID(f.projectA.ID).
		SetScopes([]string{"read_requests"}).Save(ctx)
	require.NoError(t, err)

	f.sysAdmin = newUser("admin@sys.test")
	require.NoError(t, client.User.UpdateOneID(f.sysAdmin.ID).SetScopes([]string{"read_channels"}).Exec(ctx))

	return f
}

// callerCtx builds the context the JWT and project middlewares produce for a user.
func (f *playgroundFixture) callerCtx(t *testing.T, u *ent.User, projectID int) context.Context {
	t.Helper()

	loaded, err := f.client.User.Query().Where(user.IDEQ(u.ID)).WithRoles().WithProjectUsers().Only(f.ctx)
	require.NoError(t, err)

	ctx := authz.NewUserContext(ent.NewContext(context.Background(), f.client), u.ID)
	ctx = contexts.WithUser(ctx, loaded)
	if projectID != 0 {
		ctx = contexts.WithProjectID(ctx, projectID)
	}

	return ctx
}

func requireForbidden(t *testing.T, err error) {
	t.Helper()

	coded, ok := err.(*xerrors.CodedError)
	require.True(t, ok, "expected *xerrors.CodedError, got %v", err)
	require.Equal(t, xerrors.ErrCodeForbidden, coded.Code)
}

func TestAuthorizePlayground_MemberUsesOwnProjectWithProfile(t *testing.T) {
	f := setupPlaygroundFixture(t)

	access, err := f.svc.AuthorizePlayground(f.callerCtx(t, f.memberA, f.projectA.ID), 0)
	require.NoError(t, err)
	require.NotNil(t, access.ProjectProfile)
	require.ElementsMatch(t, []int{f.allowed.ID, f.disabled.ID}, access.ProjectProfile.ChannelIDs)

	_, err = f.svc.AuthorizePlayground(f.callerCtx(t, f.memberA, f.projectA.ID), f.allowed.ID)
	require.NoError(t, err)
}

func TestAuthorizePlayground_RejectsOtherProject(t *testing.T) {
	f := setupPlaygroundFixture(t)

	_, err := f.svc.AuthorizePlayground(f.callerCtx(t, f.memberA, f.projectB.ID), 0)
	requireForbidden(t, err)
}

func TestAuthorizePlayground_RejectsMissingProjectForMember(t *testing.T) {
	f := setupPlaygroundFixture(t)

	_, err := f.svc.AuthorizePlayground(f.callerCtx(t, f.memberA, 0), 0)
	requireForbidden(t, err)
}

func TestAuthorizePlayground_RejectsMemberWithoutWriteScope(t *testing.T) {
	f := setupPlaygroundFixture(t)

	_, err := f.svc.AuthorizePlayground(f.callerCtx(t, f.viewerA, f.projectA.ID), 0)
	requireForbidden(t, err)
}

func TestAuthorizePlayground_RejectsChannelOutsideProjectOrDisabled(t *testing.T) {
	f := setupPlaygroundFixture(t)

	_, err := f.svc.AuthorizePlayground(f.callerCtx(t, f.memberA, f.projectA.ID), f.other.ID)
	requireForbidden(t, err)

	_, err = f.svc.AuthorizePlayground(f.callerCtx(t, f.memberA, f.projectA.ID), f.disabled.ID)
	requireForbidden(t, err)

	_, err = f.svc.AuthorizePlayground(f.callerCtx(t, f.memberA, f.projectA.ID), 999999)
	requireForbidden(t, err)
}

func TestAuthorizePlayground_SystemChannelReaderIsUnrestricted(t *testing.T) {
	f := setupPlaygroundFixture(t)

	access, err := f.svc.AuthorizePlayground(f.callerCtx(t, f.sysAdmin, 0), f.disabled.ID)
	require.NoError(t, err)
	require.Nil(t, access.ProjectProfile)

	access, err = f.svc.AuthorizePlayground(f.callerCtx(t, f.sysAdmin, f.projectB.ID), f.other.ID)
	require.NoError(t, err)
	require.Nil(t, access.ProjectProfile)
}
