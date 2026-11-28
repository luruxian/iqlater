// 职责: 验证项目所有者代签与管控员工个人 Key、额度只由所有者设定、默认模板自动套用。
// 关联文件:
//   internal/server/biz/api_key_issue.go
//   internal/server/biz/api_key_limits.go
//   internal/server/biz/api_key.go
//   internal/server/biz/api_key_profile_template.go
// 遵循的 Rules:
//   .agents/rules/go-general.md

package biz

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/looplj/axonhub/internal/authz"
	"github.com/looplj/axonhub/internal/contexts"
	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/ent/apikey"
	"github.com/looplj/axonhub/internal/ent/role"
	"github.com/looplj/axonhub/internal/ent/user"
	"github.com/looplj/axonhub/internal/objects"
	"github.com/looplj/axonhub/internal/pkg/xcache"
)

type keyOwnerFixture struct {
	keys      *APIKeyService
	templates *APIKeyProfileTemplateService
	client    *ent.Client
	setupCtx  context.Context
	project   *ent.Project
	other     *ent.Project
	owner     *ent.User
	employee  *ent.User
	colleague *ent.User
	outsider  *ent.User
}

func setupKeyOwnerFixture(t *testing.T) *keyOwnerFixture {
	t.Helper()

	keys, client := setupTestAPIKeyService(t, xcache.Config{Mode: xcache.ModeMemory})
	t.Cleanup(func() {
		keys.Stop()
		_ = client.Close()
	})

	ctx := authz.WithTestBypass(ent.NewContext(context.Background(), client))
	f := &keyOwnerFixture{
		keys:      keys,
		templates: &APIKeyProfileTemplateService{AbstractService: &AbstractService{db: client}},
		client:    client,
		setupCtx:  ctx,
	}

	var err error
	f.project, err = client.Project.Create().SetName("customer").Save(ctx)
	require.NoError(t, err)
	f.other, err = client.Project.Create().SetName("other-customer").Save(ctx)
	require.NoError(t, err)

	devRole, err := client.Role.Create().SetName("Developer").SetLevel(role.LevelProject).
		SetProjectID(f.project.ID).SetScopes([]string{"read_api_keys", "write_api_keys", "read_requests", "write_requests"}).Save(ctx)
	require.NoError(t, err)

	newMember := func(email string, p *ent.Project, owner bool) *ent.User {
		u, err := client.User.Create().SetEmail(email).SetPassword("x").SetStatus(user.StatusActivated).Save(ctx)
		require.NoError(t, err)
		_, err = client.UserProject.Create().SetUserID(u.ID).SetProjectID(p.ID).SetIsOwner(owner).Save(ctx)
		require.NoError(t, err)
		if !owner && p.ID == f.project.ID {
			require.NoError(t, client.User.UpdateOneID(u.ID).AddRoleIDs(devRole.ID).Exec(ctx))
		}

		return u
	}

	f.owner = newMember("owner@customer.test", f.project, true)
	f.employee = newMember("employee@customer.test", f.project, false)
	f.colleague = newMember("colleague@customer.test", f.project, false)
	f.outsider = newMember("outsider@other.test", f.other, false)

	return f
}

// as returns the context the JWT and project middlewares build for a signed-in user.
func (f *keyOwnerFixture) as(t *testing.T, u *ent.User) context.Context {
	t.Helper()

	loaded, err := f.client.User.Query().Where(user.IDEQ(u.ID)).WithRoles().WithProjectUsers().Only(f.setupCtx)
	require.NoError(t, err)

	ctx := authz.NewUserContext(ent.NewContext(context.Background(), f.client), u.ID)
	ctx = contexts.WithUser(ctx, loaded)

	return contexts.WithProjectID(ctx, f.project.ID)
}

func (f *keyOwnerFixture) defaultTemplate(t *testing.T) *ent.APIKeyProfileTemplate {
	t.Helper()

	apply := true
	tpl, err := f.templates.CreateTemplate(f.as(t, f.owner), ent.CreateAPIKeyProfileTemplateInput{
		Name:                   "employee-default",
		ProjectID:              f.project.ID,
		ApplyToNewPersonalKeys: &apply,
	}, &objects.APIKeyProfile{
		ModelIDs: []string{"sub2api-openai/gpt-6.1-sol"},
		Quota: &objects.APIKeyQuota{
			TotalTokens: new(int64(100000)),
			Period:      objects.APIKeyQuotaPeriod{Type: objects.APIKeyQuotaPeriodTypeAllTime},
		},
	})
	require.NoError(t, err)

	return tpl
}

func requireActiveTemplate(t *testing.T, key *ent.APIKey, tpl *ent.APIKeyProfileTemplate) {
	t.Helper()

	active := key.GetActiveProfile()
	require.NotNil(t, active, "the default template must become the active profile")
	require.NotNil(t, active.TemplateID)
	require.Equal(t, tpl.ID, *active.TemplateID)
	require.Equal(t, []string{"sub2api-openai/gpt-6.1-sol"}, active.ModelIDs)
}

func TestProjectOwnerIssuesPersonalKeyForMember(t *testing.T) {
	f := setupKeyOwnerFixture(t)
	tpl := f.defaultTemplate(t)

	key, err := f.keys.CreatePersonalAPIKeyForUser(f.as(t, f.owner), f.project.ID, f.employee.ID, "laptop")
	require.NoError(t, err)
	require.Equal(t, apikey.TypePersonal, key.Type)
	require.Equal(t, f.employee.ID, key.UserID, "the key belongs to the employee, not the issuer")
	require.NotEmpty(t, key.Key)
	requireActiveTemplate(t, key, tpl)

	// Afterwards only the employee can reveal the secret.
	revealed, err := f.keys.RevealAPIKey(f.as(t, f.owner), key)
	require.NoError(t, err)
	require.Empty(t, revealed)
	revealed, err = f.keys.RevealAPIKey(f.as(t, f.employee), key)
	require.NoError(t, err)
	require.Equal(t, key.Key, revealed)
}

func TestOnlyProjectOwnerIssuesKeysForActiveMembers(t *testing.T) {
	f := setupKeyOwnerFixture(t)

	_, err := f.keys.CreatePersonalAPIKeyForUser(f.as(t, f.employee), f.project.ID, f.colleague.ID, "k")
	require.ErrorContains(t, err, "only project owners")

	_, err = f.keys.CreatePersonalAPIKeyForUser(f.as(t, f.owner), f.project.ID, f.outsider.ID, "k")
	require.ErrorContains(t, err, "not a member of this project")

	_, err = f.keys.CreatePersonalAPIKeyForUser(f.as(t, f.owner), f.other.ID, f.outsider.ID, "k")
	require.ErrorContains(t, err, "only project owners")

	require.NoError(t, f.client.User.UpdateOneID(f.colleague.ID).SetStatus(user.StatusDeactivated).Exec(f.setupCtx))
	_, err = f.keys.CreatePersonalAPIKeyForUser(f.as(t, f.owner), f.project.ID, f.colleague.ID, "k")
	require.ErrorContains(t, err, "not activated")
}

func TestEmployeePersonalKeyGetsDefaultLimitsAndCannotChangeThem(t *testing.T) {
	f := setupKeyOwnerFixture(t)
	tpl := f.defaultTemplate(t)
	personal := apikey.TypePersonal

	key, err := f.keys.CreateAPIKey(f.as(t, f.employee), ent.CreateAPIKeyInput{
		Name:      "mine",
		Type:      &personal,
		ProjectID: f.project.ID,
	})
	require.NoError(t, err)
	requireActiveTemplate(t, key, tpl)

	_, err = f.keys.UpdateAPIKeyProfiles(f.as(t, f.employee), key.ID, objects.APIKeyProfiles{
		ActiveProfile: "free",
		Profiles:      []objects.APIKeyProfile{{Name: "free"}},
	})
	require.ErrorContains(t, err, "only project owners can change API key limits")

	_, err = f.templates.LoadTemplate(f.as(t, f.employee), tpl.ID, key.ID)
	require.ErrorContains(t, err, "only project owners can change API key limits")

	raised := objects.APIKeyProfiles{
		ActiveProfile: "raised",
		Profiles: []objects.APIKeyProfile{{
			Name: "raised",
			Quota: &objects.APIKeyQuota{
				Cost:   new(decimal.NewFromInt(50)),
				Period: objects.APIKeyQuotaPeriod{Type: objects.APIKeyQuotaPeriodTypeAllTime},
			},
		}},
	}
	updated, err := f.keys.UpdateAPIKeyProfiles(f.as(t, f.owner), key.ID, raised)
	require.NoError(t, err)
	require.Equal(t, "raised", updated.GetActiveProfile().Name)
}

func TestMembersCannotManageTemplates(t *testing.T) {
	f := setupKeyOwnerFixture(t)
	tpl := f.defaultTemplate(t)

	_, err := f.templates.CreateTemplate(f.as(t, f.employee), ent.CreateAPIKeyProfileTemplateInput{
		Name: "loose", ProjectID: f.project.ID,
	}, &objects.APIKeyProfile{})
	require.ErrorContains(t, err, "only project owners can change API key limits")

	name := "renamed"
	_, err = f.templates.UpdateTemplate(f.as(t, f.employee), tpl.ID, ent.UpdateAPIKeyProfileTemplateInput{Name: &name}, nil)
	require.ErrorContains(t, err, "only project owners can change API key limits")

	_, err = f.templates.DeleteTemplate(f.as(t, f.employee), tpl.ID)
	require.ErrorContains(t, err, "only project owners can change API key limits")
}

func TestOnlyOneTemplateAppliesToNewPersonalKeys(t *testing.T) {
	f := setupKeyOwnerFixture(t)
	first := f.defaultTemplate(t)

	apply := true
	second, err := f.templates.CreateTemplate(f.as(t, f.owner), ent.CreateAPIKeyProfileTemplateInput{
		Name: "second", ProjectID: f.project.ID, ApplyToNewPersonalKeys: &apply,
	}, &objects.APIKeyProfile{})
	require.NoError(t, err)
	require.True(t, second.ApplyToNewPersonalKeys)

	first, err = f.client.APIKeyProfileTemplate.Get(f.setupCtx, first.ID)
	require.NoError(t, err)
	require.False(t, first.ApplyToNewPersonalKeys)

	_, err = f.templates.UpdateTemplate(f.as(t, f.owner), first.ID, ent.UpdateAPIKeyProfileTemplateInput{ApplyToNewPersonalKeys: &apply}, nil)
	require.NoError(t, err)
	second, err = f.client.APIKeyProfileTemplate.Get(f.setupCtx, second.ID)
	require.NoError(t, err)
	require.False(t, second.ApplyToNewPersonalKeys)
}

func TestProjectOwnerDisablesEmployeePersonalKey(t *testing.T) {
	f := setupKeyOwnerFixture(t)
	key, err := f.keys.CreatePersonalAPIKeyForUser(f.as(t, f.owner), f.project.ID, f.employee.ID, "laptop")
	require.NoError(t, err)

	_, err = f.keys.UpdateAPIKeyStatus(f.as(t, f.colleague), key.ID, apikey.StatusDisabled)
	require.Error(t, err, "a colleague cannot touch someone else's personal key")

	updated, err := f.keys.UpdateAPIKeyStatus(f.as(t, f.owner), key.ID, apikey.StatusDisabled)
	require.NoError(t, err)
	require.Equal(t, apikey.StatusDisabled, updated.Status)

	require.NoError(t, f.keys.BulkArchiveAPIKeys(f.as(t, f.owner), []int{key.ID}))
	archived, err := f.client.APIKey.Get(f.setupCtx, key.ID)
	require.NoError(t, err)
	require.Equal(t, apikey.StatusArchived, archived.Status)
}

func TestServiceAccountLimitsFollowItsCreator(t *testing.T) {
	f := setupKeyOwnerFixture(t)

	// Service accounts cannot read personal keys; the exposed targets are shared project keys.
	target, err := f.client.APIKey.Create().SetName("shared").SetKey("ah-shared-target").
		SetUserID(f.owner.ID).SetProjectID(f.project.ID).SetType(apikey.TypeUser).Save(f.setupCtx)
	require.NoError(t, err)

	newServiceAccount := func(creator *ent.User, name string) context.Context {
		sa, err := f.client.APIKey.Create().SetName(name).SetKey("ah-sa-"+name).
			SetUserID(creator.ID).SetProjectID(f.project.ID).SetType(apikey.TypeServiceAccount).
			SetScopes([]string{"read_api_keys", "write_api_keys"}).Save(f.setupCtx)
		require.NoError(t, err)

		// The OpenAPI middleware sets an API key principal and no user.
		ctx := authz.NewAPIKeyContext(ent.NewContext(context.Background(), f.client), sa.ID, f.project.ID)
		ctx = contexts.WithAPIKey(ctx, sa)

		return contexts.WithProjectID(ctx, f.project.ID)
	}

	profiles := objects.APIKeyProfiles{ActiveProfile: "free", Profiles: []objects.APIKeyProfile{{Name: "free"}}}

	_, err = f.keys.UpdateAPIKeyProfiles(newServiceAccount(f.employee, "employee"), target.ID, profiles)
	require.ErrorContains(t, err, "only project owners can change API key limits")

	_, err = f.keys.UpdateAPIKeyProfiles(newServiceAccount(f.owner, "owner"), target.ID, profiles)
	require.NoError(t, err)
}
