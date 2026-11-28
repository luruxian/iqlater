package biz

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/looplj/axonhub/internal/authz"
	"github.com/looplj/axonhub/internal/contexts"
	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/ent/enttest"
	"github.com/looplj/axonhub/internal/ent/request"
	"github.com/looplj/axonhub/internal/pkg/xcache"
	"github.com/looplj/axonhub/llm"
	"github.com/looplj/axonhub/llm/httpclient"
)

func TestRequestServiceCreateRequestAssignsPlaygroundUser(t *testing.T) {
	client := enttest.NewEntClient(t, "sqlite3", "file:request_creator?mode=memory&_fk=0")
	t.Cleanup(func() { client.Close() })

	ctx := authz.WithTestBypass(ent.NewContext(context.Background(), client))
	systemService := NewSystemService(SystemServiceParams{Ent: client})
	channelService := NewChannelServiceForTest(client)
	usageLogService := NewUsageLogService(client, systemService, channelService)
	dataStorageService := NewDataStorageService(DataStorageServiceParams{
		SystemService: systemService,
		CacheConfig:   xcache.Config{Mode: xcache.ModeMemory},
		Client:        client,
	})
	requestService := NewRequestService(client, systemService.CacheConfig, systemService, usageLogService, dataStorageService, NewLiveStreamRegistry())

	ctx = contexts.WithProjectID(ctx, 100)
	ctx = contexts.WithUser(ctx, &ent.User{ID: 42})
	playgroundRequest, err := requestService.CreateRequest(
		contexts.WithSource(ctx, request.SourcePlayground),
		&llm.Request{Model: "playground-model"},
		&httpclient.Request{JSONBody: []byte(`{"model":"playground-model"}`)},
		llm.APIFormatOpenAIChatCompletion,
	)
	require.NoError(t, err)
	require.NotNil(t, playgroundRequest.UserID)
	require.Equal(t, 42, *playgroundRequest.UserID)

	apiRequest, err := requestService.CreateRequest(
		contexts.WithSource(ctx, request.SourceAPI),
		&llm.Request{Model: "api-model"},
		&httpclient.Request{JSONBody: []byte(`{"model":"api-model"}`)},
		llm.APIFormatOpenAIChatCompletion,
	)
	require.NoError(t, err)
	require.Nil(t, apiRequest.UserID)

	// API traffic is attributed to the key's creator, not to a signed-in user.
	keyRequest, err := requestService.CreateRequest(
		contexts.WithAPIKey(contexts.WithSource(ctx, request.SourceAPI), &ent.APIKey{ID: 7, UserID: 99, ProjectID: 100}),
		&llm.Request{Model: "api-model"},
		&httpclient.Request{JSONBody: []byte(`{"model":"api-model"}`)},
		llm.APIFormatOpenAIChatCompletion,
	)
	require.NoError(t, err)
	require.NotNil(t, keyRequest.UserID)
	require.Equal(t, 99, *keyRequest.UserID)
}
