// 职责: 校验调用者能否在当前项目使用 Playground 及指定渠道，并提供按项目 profile 过滤渠道的共用逻辑。
// 关联文件:
//   internal/server/api/playground.go
//   internal/server/gql/axonhub.resolvers.go
//   internal/authz/scope.go
// 遵循的 Rules:
//   .agents/rules/go-general.md
//   .agents/rules/biz-services.md
// 注意事项:
//   Playground 在系统 bypass 下执行编排，项目与渠道的授权只能在进入编排前完成。
//   可见性规则须与 AllChannelSummarys 保持一致：系统级 read_channels 可用任意渠道，
//   其他人须是项目成员且有 write_requests 或 write_api_keys，只能用已启用且符合项目 profile 的渠道。

package biz

import (
	"context"
	"fmt"

	"github.com/samber/lo"

	"github.com/looplj/axonhub/internal/authz"
	"github.com/looplj/axonhub/internal/contexts"
	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/ent/channel"
	"github.com/looplj/axonhub/internal/objects"
	"github.com/looplj/axonhub/internal/pkg/xerrors"
	"github.com/looplj/axonhub/internal/scopes"
)

// PlaygroundAccess is the result of a successful playground authorization.
type PlaygroundAccess struct {
	// ProjectProfile limits model routing for callers without system channel access.
	// Nil means routing is not narrowed.
	ProjectProfile *objects.ProjectProfile
}

// ChannelMatchesProjectProfile reports whether a channel is allowed by a project profile.
func ChannelMatchesProjectProfile(ch *ent.Channel, projectProfile *objects.ProjectProfile) bool {
	if projectProfile == nil {
		return true
	}

	if len(projectProfile.ChannelIDs) > 0 && !lo.Contains(projectProfile.ChannelIDs, ch.ID) {
		return false
	}

	if len(projectProfile.ChannelTags) > 0 && !projectProfile.MatchChannelTags(ch.Tags) {
		return false
	}

	return true
}

// FilterChannelsByProjectProfile keeps the channels a project profile allows.
func FilterChannelsByProjectProfile(channels []*ent.Channel, projectProfile *objects.ProjectProfile) []*ent.Channel {
	if projectProfile == nil {
		return channels
	}

	return lo.Filter(channels, func(ch *ent.Channel, _ int) bool {
		return ChannelMatchesProjectProfile(ch, projectProfile)
	})
}

// AuthorizePlayground checks that the caller may use the playground in the project
// selected in ctx and, when channelID is non-zero, that the channel is usable there.
func (svc *ChannelService) AuthorizePlayground(ctx context.Context, channelID int) (*PlaygroundAccess, error) {
	canReadChannels := authz.HasScope(ctx, scopes.ScopeReadChannels)

	projectID, ok := contexts.GetProjectID(ctx)
	if !ok || projectID == 0 {
		if !canReadChannels {
			return nil, xerrors.ForbiddenError("permission denied: select a project to use the playground")
		}

		return &PlaygroundAccess{}, nil
	}

	if canReadChannels {
		return &PlaygroundAccess{}, nil
	}

	if !authz.HasScope(ctx, scopes.ScopeWriteRequests) && !authz.HasScope(ctx, scopes.ScopeWriteAPIKeys) {
		return nil, xerrors.ForbiddenError("permission denied: no playground access in this project")
	}

	client := svc.entFromContext(ctx)

	// The caller was authorized above; these host reads only load the routing boundary.
	project, err := authz.RunAsHost(ctx, client, "playground-project-profile", func(ctx context.Context) (*ent.Project, error) {
		return client.Project.Get(ctx, projectID)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get project: %w", err)
	}

	access := &PlaygroundAccess{ProjectProfile: project.GetActiveProfile()}

	if channelID == 0 {
		return access, nil
	}

	ch, err := authz.RunAsHost(ctx, client, "playground-channel", func(ctx context.Context) (*ent.Channel, error) {
		return client.Channel.Get(ctx, channelID)
	})
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, xerrors.ForbiddenError("permission denied: channel is not available in this project")
		}

		return nil, fmt.Errorf("failed to get channel: %w", err)
	}

	if ch.Status != channel.StatusEnabled || !ChannelMatchesProjectProfile(ch, access.ProjectProfile) {
		return nil, xerrors.ForbiddenError("permission denied: channel is not available in this project")
	}

	return access, nil
}
