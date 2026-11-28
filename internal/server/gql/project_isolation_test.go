// 职责: 经真实的 /admin/graphql 链路验证两个客户（项目）之间的数据隔离与员工管理边界。
// 关联文件:
//   internal/server/gql/plugin_endpoint_test.go
//   internal/server/gql/project_members.resolvers.go
//   internal/scopes/rule_user_project_scope.go
//   internal/server/biz/api_key_limits.go
// 遵循的 Rules:
//   .agents/rules/go-general.md

package gql

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/ent/apikey"
	"github.com/looplj/axonhub/internal/ent/project"
	"github.com/looplj/axonhub/internal/ent/request"
	"github.com/looplj/axonhub/internal/ent/user"
	"github.com/looplj/axonhub/internal/objects"
)

type customerOrg struct {
	project     *ent.Project
	owner       *ent.User
	employee    *ent.User
	ownerJWT    string
	employeeJWT string
	key         *ent.APIKey
	request     *ent.Request
	usage       *ent.UsageLog
}

func gid(typ string, id int) string { return fmt.Sprintf("gid://axonhub/%s/%d", typ, id) }

func newCustomerOrg(t *testing.T, fx *pluginEndpointFixture, name string) *customerOrg {
	t.Helper()

	ctx := fx.setupCtx
	org := &customerOrg{}
	org.project = fx.db.Project.Create().SetName(name).SetStatus(project.StatusActive).SaveX(ctx)

	newUser := func(email string, owner bool, scopes []string) *ent.User {
		u := fx.db.User.Create().SetEmail(email).SetPassword("x").SetFirstName(email).
			SetStatus(user.StatusActivated).SaveX(ctx)
		fx.db.UserProject.Create().SetUserID(u.ID).SetProjectID(org.project.ID).
			SetIsOwner(owner).SetScopes(scopes).SaveX(ctx)

		return u
	}
	org.owner = newUser("owner@"+name+".test", true, nil)
	org.employee = newUser("employee@"+name+".test", false,
		[]string{"read_api_keys", "write_api_keys", "read_requests", "write_requests", "read_users"})

	jwt := func(u *ent.User) string {
		signed, err := fx.auth.GenerateJWTToken(ctx, u)
		require.NoError(t, err)

		return signed
	}
	org.ownerJWT = jwt(org.owner)
	org.employeeJWT = jwt(org.employee)

	org.key = fx.db.APIKey.Create().SetName("employee key").SetKey("ah-" + name + "-employee").
		SetUserID(org.employee.ID).SetProjectID(org.project.ID).SetType(apikey.TypePersonal).SaveX(ctx)
	org.request = fx.db.Request.Create().SetProjectID(org.project.ID).SetAPIKeyID(org.key.ID).
		SetModelID("m").SetSource(request.SourceAPI).SetStatus(request.StatusCompleted).
		SetRequestBody(objects.JSONRawMessage(`{"secret":"` + name + `"}`)).
		SetUserID(org.employee.ID).SaveX(ctx)
	org.usage = fx.db.UsageLog.Create().SetRequestID(org.request.ID).SetProjectID(org.project.ID).
		SetAPIKeyID(org.key.ID).SetUserID(org.employee.ID).SetModelID("m").SetTotalTokens(10).SaveX(ctx)

	return org
}

// postAs sends an admin GraphQL request with the X-Project-ID the frontend sets.
func postAs(t *testing.T, fx *pluginEndpointFixture, token string, projectID int, query string) (map[string]any, string) {
	t.Helper()

	encoded, err := json.Marshal(map[string]any{"query": query})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/admin/graphql", bytes.NewReader(encoded))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Project-ID", gid("Project", projectID))

	recorder := httptest.NewRecorder()
	fx.engine.ServeHTTP(recorder, req)

	var payload map[string]any
	_ = json.Unmarshal(recorder.Body.Bytes(), &payload)

	return payload, recorder.Body.String()
}

func TestCustomerOrgsCannotReadEachOthersData(t *testing.T) {
	fx := newPluginEndpointFixture(t, "customer_org_isolation_read")
	a := newCustomerOrg(t, fx, "alpha")
	b := newCustomerOrg(t, fx, "beta")

	readsFor := func(o *customerOrg) map[string]string {
		return map[string]string{
			"api key node": fmt.Sprintf(`{ node(id: %q) { ... on APIKey { id name } } }`, gid("APIKey", o.key.ID)),
			"user node":    fmt.Sprintf(`{ node(id: %q) { ... on User { id email } } }`, gid("User", o.employee.ID)),
			"request node": fmt.Sprintf(`{ node(id: %q) { ... on Request { id } } }`, gid("Request", o.request.ID)),
			"usage node":   fmt.Sprintf(`{ node(id: %q) { ... on UsageLog { id } } }`, gid("UsageLog", o.usage.ID)),
			"api keys":     `{ apiKeys(first: 50) { edges { node { id name } } } }`,
			"users":        `{ users(first: 50) { edges { node { id email } } } }`,
			"requests":     `{ requests(first: 50) { edges { node { id } } } }`,
			"usage logs":   `{ usageLogs(first: 50) { edges { node { id } } } }`,
		}
	}

	// Control: the same queries return the owner's own data, so an empty answer below means isolation.
	ownExpect := map[string]string{
		"api key node": gid("APIKey", a.key.ID), "user node": a.employee.Email,
		"request node": gid("Request", a.request.ID), "usage node": gid("UsageLog", a.usage.ID),
		"api keys": gid("APIKey", a.key.ID), "users": a.employee.Email,
		"requests": gid("Request", a.request.ID), "usage logs": gid("UsageLog", a.usage.ID),
	}
	for name, query := range readsFor(a) {
		payload, raw := postAs(t, fx, a.ownerJWT, a.project.ID, query)
		require.NotContains(t, payload, "errors", "%s: %s", name, raw)
		require.Contains(t, raw, ownExpect[name], "%s: %s", name, raw)
	}

	reads := readsFor(b)
	reads["project"] = fmt.Sprintf(`{ node(id: %q) { ... on Project { id name } } }`, gid("Project", b.project.ID))

	leaks := []string{b.employee.Email, b.owner.Email, "ah-beta-employee", `"beta"`,
		gid("APIKey", b.key.ID), gid("Request", b.request.ID), gid("UsageLog", b.usage.ID)}

	for _, caller := range []struct {
		name  string
		token string
	}{{"owner", a.ownerJWT}, {"employee", a.employeeJWT}} {
		// Asking in their own project, and pointing X-Project-ID at the other customer.
		for _, projectID := range []int{a.project.ID, b.project.ID} {
			for name, query := range reads {
				_, raw := postAs(t, fx, caller.token, projectID, query)
				for _, leak := range leaks {
					require.NotContains(t, raw, leak, "%s of alpha (project %d) read %s: %s", caller.name, projectID, name, raw)
				}
			}
		}
	}

}

func TestCustomerOwnerCannotManageAnotherCustomer(t *testing.T) {
	fx := newPluginEndpointFixture(t, "customer_org_isolation_write")
	a := newCustomerOrg(t, fx, "alpha")
	b := newCustomerOrg(t, fx, "beta")

	writes := []string{
		fmt.Sprintf(`mutation { updateAPIKeyStatus(id: %q, status: disabled) { id } }`, gid("APIKey", b.key.ID)),
		fmt.Sprintf(`mutation { updateUserStatus(id: %q, status: deactivated) { id } }`, gid("User", b.employee.ID)),
		fmt.Sprintf(`mutation { createPersonalAPIKeyForUser(input: {projectID: %q, userID: %q, name: "x"}) { key } }`,
			gid("Project", b.project.ID), gid("User", b.employee.ID)),
		fmt.Sprintf(`mutation { createPersonalAPIKeyForUser(input: {projectID: %q, userID: %q, name: "x"}) { key } }`,
			gid("Project", a.project.ID), gid("User", b.employee.ID)),
		fmt.Sprintf(`mutation { removeUserFromProject(input: {projectId: %q, userId: %q}) }`,
			gid("Project", b.project.ID), gid("User", b.employee.ID)),
	}

	for _, projectID := range []int{a.project.ID, b.project.ID} {
		for _, mutation := range writes {
			payload, raw := postAs(t, fx, a.ownerJWT, projectID, mutation)
			require.Contains(t, payload, "errors", "alpha's owner changed beta via %s: %s", mutation, raw)
		}
	}

	key := fx.db.APIKey.GetX(fx.setupCtx, b.key.ID)
	require.Equal(t, apikey.StatusEnabled, key.Status)
	employee := fx.db.User.GetX(fx.setupCtx, b.employee.ID)
	require.Equal(t, user.StatusActivated, employee.Status)
	require.Equal(t, 1, fx.db.APIKey.Query().Where(apikey.UserIDEQ(b.employee.ID)).CountX(fx.setupCtx))
}

func TestCustomerOwnerManagesOwnEmployees(t *testing.T) {
	fx := newPluginEndpointFixture(t, "customer_org_own_management")
	a := newCustomerOrg(t, fx, "alpha")

	payload, raw := postAs(t, fx, a.ownerJWT, a.project.ID, fmt.Sprintf(
		`mutation { createPersonalAPIKeyForUser(input: {projectID: %q, userID: %q, name: "issued"}) { key apiKey { id } } }`,
		gid("Project", a.project.ID), gid("User", a.employee.ID)))
	require.NotContains(t, payload, "errors", raw)
	require.Contains(t, raw, `"key":"ah-`)

	payload, raw = postAs(t, fx, a.ownerJWT, a.project.ID, fmt.Sprintf(
		`mutation { updateAPIKeyStatus(id: %q, status: disabled) { id status } }`, gid("APIKey", a.key.ID)))
	require.NotContains(t, payload, "errors", raw)

	payload, raw = postAs(t, fx, a.ownerJWT, a.project.ID, fmt.Sprintf(
		`mutation { updateUserStatus(id: %q, status: deactivated) { id status } }`, gid("User", a.employee.ID)))
	require.NotContains(t, payload, "errors", raw)
	require.Equal(t, user.StatusDeactivated, fx.db.User.GetX(fx.setupCtx, a.employee.ID).Status)

	// The employee cannot issue keys for colleagues.
	b := newCustomerOrg(t, fx, "beta")
	payload, raw = postAs(t, fx, b.employeeJWT, b.project.ID, fmt.Sprintf(
		`mutation { createPersonalAPIKeyForUser(input: {projectID: %q, userID: %q, name: "x"}) { key } }`,
		gid("Project", b.project.ID), gid("User", b.owner.ID)))
	require.Contains(t, payload, "errors", raw)
}
