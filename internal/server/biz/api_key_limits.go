// 职责: 判断调用者能否修改 API Key 的额度/模型限制，以及能否管理他人的个人 Key。
// 关联文件:
//   internal/server/biz/api_key.go
//   internal/server/biz/api_key_profile_template.go
//   internal/scopes/plugin.go
// 遵循的 Rules:
//   .agents/rules/go-general.md
//   .agents/rules/biz-services.md
// 注意事项:
//   Key 的 profile（额度、可用模型、渠道）只允许系统所有者、系统级 write_api_keys 持有者
//   和 Key 所在项目的项目所有者修改；普通成员即使有项目级 write_api_keys 也不行。
//   OpenAPI 调用没有登录用户，按 service account Key 的创建人判断；不能仅凭 Key 的 write_api_keys
//   放行，否则成员可借自建的 service account 绕过限制。

package biz

import (
	"context"
	"fmt"

	"github.com/samber/lo"

	"github.com/looplj/axonhub/internal/authz"
	"github.com/looplj/axonhub/internal/contexts"
	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/ent/apikey"
	"github.com/looplj/axonhub/internal/ent/user"
	"github.com/looplj/axonhub/internal/pkg/xerrors"
	"github.com/looplj/axonhub/internal/scopes"
)

// loadCallerWithMemberships reloads the context user with the edges ownership checks need.
func loadCallerWithMemberships(ctx context.Context, client *ent.Client) (*ent.User, error) {
	caller, ok := contexts.GetUser(ctx)
	if !ok || caller == nil {
		return nil, xerrors.ForbiddenError("permission denied: a signed-in user is required")
	}

	return loadUserWithMemberships(ctx, client, caller.ID)
}

func loadUserWithMemberships(ctx context.Context, client *ent.Client, userID int) (*ent.User, error) {
	return authz.RunAsHost(ctx, client, "api-key-limits-caller", func(hostCtx context.Context) (*ent.User, error) {
		return client.User.Query().Where(user.IDEQ(userID)).WithRoles().WithProjectUsers().Only(hostCtx)
	})
}

// limitsActor returns the user whose authority applies to a limits change: the
// signed-in user, or for OpenAPI calls the creator of the service-account key.
func limitsActor(ctx context.Context, client *ent.Client) (*ent.User, error) {
	if _, ok := contexts.GetUser(ctx); ok {
		return loadCallerWithMemberships(ctx, client)
	}

	key, ok := contexts.GetAPIKey(ctx)
	if !ok || key == nil || key.Type != apikey.TypeServiceAccount || key.UserID == 0 {
		return nil, xerrors.ForbiddenError("permission denied: a signed-in user is required")
	}

	return loadUserWithMemberships(ctx, client, key.UserID)
}

func ownsProject(u *ent.User, projectID int) bool {
	return u.IsOwner || lo.ContainsBy(u.Edges.ProjectUsers, func(m *ent.UserProject) bool {
		return m.ProjectID == projectID && m.IsOwner
	})
}

// isInternalPrincipal reports host and test contexts, which carry no end user.
func isInternalPrincipal(ctx context.Context) bool {
	p, ok := authz.GetPrincipal(ctx)
	return ok && (p.Type == authz.PrincipalTypeSystem || p.Type == authz.PrincipalTypeTest)
}

// requireAPIKeyLimitsManager allows changing API key limits only for system owners,
// system-level write_api_keys holders and owners of the given project.
func requireAPIKeyLimitsManager(ctx context.Context, client *ent.Client, projectID int) error {
	if _, ok := contexts.GetUser(ctx); !ok && isInternalPrincipal(ctx) {
		return nil
	}

	actor, err := limitsActor(ctx, client)
	if err != nil {
		return err
	}

	if ownsProject(actor, projectID) || scopes.EffectiveSystemScope(ctx, actor, scopes.ScopeWriteAPIKeys) {
		return nil
	}

	return xerrors.ForbiddenError("permission denied: only project owners can change API key limits")
}

// requirePersonalAPIKeyManager allows managing a personal key for its creator,
// system owners and owners of the key's project.
func requirePersonalAPIKeyManager(ctx context.Context, client *ent.Client, key *ent.APIKey) error {
	caller, ok := contexts.GetUser(ctx)
	if !ok || caller == nil {
		return fmt.Errorf("user not found in context")
	}
	if key.UserID == caller.ID || caller.IsOwner {
		return nil
	}

	loaded, err := loadCallerWithMemberships(ctx, client)
	if err != nil {
		return err
	}
	if ownsProject(loaded, key.ProjectID) {
		return nil
	}

	return fmt.Errorf("personal API key can only be modified by its creator, a project owner or a system owner")
}
