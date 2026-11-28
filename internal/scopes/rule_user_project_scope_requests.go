package scopes

import (
	"context"

	"entgo.io/ent/dialect/sql/sqlgraph"
	"entgo.io/ent/entql"

	"github.com/looplj/axonhub/internal/contexts"
	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/ent/apikey"
	"github.com/looplj/axonhub/internal/ent/privacy"
	"github.com/looplj/axonhub/internal/ent/request"
)

// UserProjectScopeReadRequestsRule restricts project request and usage log reads.
// Members see only their own requests (Playground or via API keys they created);
// project owners and system-scoped users can audit all records in the project.
func UserProjectScopeReadRequestsRule(requiredScope ScopeSlug) privacy.QueryRule {
	return privacy.FilterFunc(func(ctx context.Context, q privacy.Filter) error {
		projectID, hasProjectID := contexts.GetProjectID(ctx)
		if !hasProjectID {
			return privacy.Skipf("Project ID not found in context")
		}

		currentUser, err := getUserFromContext(ctx)
		if err != nil {
			return privacy.Skipf("User not found in context")
		}

		if !EffectiveSystemScope(ctx, currentUser, requiredScope) && !EffectiveProjectScope(ctx, currentUser, projectID, requiredScope) {
			return privacy.Skipf("User %d can not query project %d with scope %s", currentUser.ID, projectID, requiredScope)
		}

		if pf, ok := q.(ProjectOwnedFilter); ok {
			pf.WhereProjectID(entql.IntEQ(projectID))
		} else {
			return privacy.Skipf("Not a project-owned query")
		}

		if EffectiveSystemScope(ctx, currentUser, requiredScope) || userIsProjectOwner(currentUser, projectID) {
			return privacy.Allowf("User %d can audit requests in project %d", currentUser.ID, projectID)
		}

		// Members see only requests attributed to them. The API key creator check covers
		// API requests stored before requests.user_id was populated.
		switch q := q.(type) {
		case *ent.RequestFilter:
			q.Where(entql.Or(
				entql.FieldEQ(request.FieldUserID, currentUser.ID),
				entql.HasEdgeWith("api_key", sqlgraph.WrapFunc(apikey.UserID(currentUser.ID))),
			))
		case *ent.UsageLogFilter:
			q.WhereHasRequestWith(request.Or(
				request.UserIDEQ(currentUser.ID),
				request.HasAPIKeyWith(apikey.UserID(currentUser.ID)),
			))
		}

		return privacy.Allowf("User %d can query own requests in project %d", currentUser.ID, projectID)
	})
}
