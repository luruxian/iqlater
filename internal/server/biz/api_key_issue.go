// 职责: 项目所有者替成员签发个人 API Key，并给新个人 Key 套用项目的默认限制模板。
// 关联文件:
//   internal/server/biz/api_key.go
//   internal/server/biz/api_key_limits.go
//   internal/server/biz/api_key_profile_template.go
//   internal/server/gql/axonhub.graphql
// 遵循的 Rules:
//   .agents/rules/go-general.md
//   .agents/rules/biz-services.md
// 注意事项:
//   代签的 Key 属于员工（user_id 为员工），明文只在创建响应里返回一次供管理员转交，
//   之后 RevealAPIKey 仍只对员工本人可见。
//   默认模板写入为激活 profile，员工自建的个人 Key 也一样，员工无权改掉。

package biz

import (
	"context"
	"fmt"

	"github.com/samber/lo"

	"github.com/looplj/axonhub/internal/authz"
	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/ent/apikey"
	"github.com/looplj/axonhub/internal/ent/apikeyprofiletemplate"
	"github.com/looplj/axonhub/internal/ent/user"
	"github.com/looplj/axonhub/internal/pkg/xerrors"
)

// applyNewPersonalKeyTemplate makes the project's default template the active
// profile of a newly created personal key. Keys are returned unchanged when the
// project has no default template.
func applyNewPersonalKeyTemplate(ctx context.Context, client *ent.Client, key *ent.APIKey) (*ent.APIKey, error) {
	// The key was just created by an authorized caller; applying the project's
	// default limits is a fixed host write.
	return authz.RunAsHost(ctx, client, "apply-new-personal-key-template", func(hostCtx context.Context) (*ent.APIKey, error) {
		template, err := client.APIKeyProfileTemplate.Query().
			Where(
				apikeyprofiletemplate.ProjectIDEQ(key.ProjectID),
				apikeyprofiletemplate.ApplyToNewPersonalKeysEQ(true),
			).
			First(hostCtx)
		if ent.IsNotFound(err) {
			return key, nil
		}
		if err != nil {
			return nil, fmt.Errorf("failed to load default personal key template: %w", err)
		}

		profiles, name, err := appendTemplateProfile(key.Profiles, template)
		if err != nil {
			return nil, err
		}
		profiles.ActiveProfile = name

		updated, err := client.APIKey.UpdateOneID(key.ID).SetProfiles(profiles).Save(hostCtx)
		if err != nil {
			return nil, fmt.Errorf("failed to apply default personal key template: %w", err)
		}

		return updated, nil
	})
}

// CreatePersonalAPIKeyForUser lets a project owner issue a personal key owned by
// an active member of the project.
func (s *APIKeyService) CreatePersonalAPIKeyForUser(ctx context.Context, projectID, userID int, name string) (*ent.APIKey, error) {
	if name == "" {
		return nil, xerrors.ValidationError("API key name is required")
	}

	// The response carries the plaintext key, so plugins acting for a user may not call this.
	if err := authz.RequireDirectUser(ctx); err != nil {
		return nil, xerrors.ForbiddenError("permission denied: a signed-in user is required")
	}

	caller, err := loadCallerWithMemberships(ctx, s.entFromContext(ctx))
	if err != nil {
		return nil, err
	}
	if !ownsProject(caller, projectID) {
		return nil, xerrors.ForbiddenError("permission denied: only project owners can issue keys for members")
	}

	generatedKey, err := GenerateAPIKey(s.keyPrefix)
	if err != nil {
		return nil, fmt.Errorf("failed to generate API key: %w", err)
	}

	var apiKey *ent.APIKey

	err = s.RunInTransaction(ctx, func(ctx context.Context) error {
		client := s.entFromContext(ctx)

		target, err := authz.RunAsHost(ctx, client, "issue-personal-key-target", func(hostCtx context.Context) (*ent.User, error) {
			return client.User.Query().Where(user.IDEQ(userID)).WithProjectUsers().Only(hostCtx)
		})
		if err != nil {
			if ent.IsNotFound(err) {
				return xerrors.NotFoundError("user")
			}

			return fmt.Errorf("failed to load key owner: %w", err)
		}
		if target.Status != user.StatusActivated {
			return xerrors.ValidationError("the user is not activated")
		}
		if !lo.ContainsBy(target.Edges.ProjectUsers, func(m *ent.UserProject) bool { return m.ProjectID == projectID }) {
			return xerrors.ValidationError("the user is not a member of this project")
		}

		if err := s.lockProjectForAPIKeyName(ctx, projectID); err != nil {
			return err
		}

		// The caller owns the project and the target is a member, checked above.
		created, err := authz.RunAsHost(ctx, client, "issue-personal-key", func(hostCtx context.Context) (*ent.APIKey, error) {
			created, err := client.APIKey.Create().
				SetName(name).
				SetKey(generatedKey).
				SetUserID(target.ID).
				SetProjectID(projectID).
				SetType(apikey.TypePersonal).
				Save(hostCtx)
			if err != nil {
				return nil, fmt.Errorf("failed to create API key: %w", err)
			}

			dupCount, err := client.APIKey.Query().Where(
				apikey.NameEQ(name),
				apikey.ProjectIDEQ(projectID),
				apikey.Or(apikey.TypeNEQ(apikey.TypePersonal), apikey.UserIDEQ(target.ID)),
			).Count(hostCtx)
			if err != nil {
				return nil, fmt.Errorf("failed to check API key name uniqueness: %w", err)
			}
			if dupCount > 1 {
				return nil, xerrors.DuplicateNameError("API Key", name)
			}

			return created, nil
		})
		if err != nil {
			return err
		}

		apiKey, err = applyNewPersonalKeyTemplate(ctx, client, created)

		return err
	})
	if err != nil {
		return nil, err
	}

	return apiKey, nil
}
