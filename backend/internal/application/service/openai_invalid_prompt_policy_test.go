package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const testOpenAIInvalidPromptPolicyBody = `{"error":{"message":"Invalid prompt: your prompt was flagged as potentially violating our usage policy. Please try again with a different prompt: https://platform.openai.com/docs/guides/reasoning#advice-on-prompting","type":"upstream_error"}}`

func TestOpenAIInvalidPromptPolicyErrorStopsAccountFailover(t *testing.T) {
	body := []byte(testOpenAIInvalidPromptPolicyBody)

	require.True(t, isOpenAIInvalidPromptPolicyError("", body))
	require.True(t, (&OpenAIGatewayService{}).shouldFailoverOpenAIUpstreamResponse(http.StatusBadGateway, "", body))

	failoverErr := newOpenAIUpstreamFailoverError(
		http.StatusBadGateway,
		http.Header{"X-Request-Id": []string{"req-invalid-prompt"}},
		body,
		"",
		true,
	)
	require.True(t, failoverErr.IsOpenAIInvalidPromptPolicyError())
	require.Equal(t, GatewayFailureScopeRequest, failoverErr.Scope)
	require.Equal(t, NextAccountStop, failoverErr.NextAccountAction)
	require.False(t, failoverErr.ShouldRetryNextAccount())
	require.False(t, failoverErr.RetryableOnSameAccount)
	require.False(t, failoverErr.ShouldReportAccountScheduleFailure())
	require.Equal(t, http.StatusBadRequest, failoverErr.ClientStatusCode)
	require.Equal(t, OpenAIInvalidPromptPolicyClientMessage, failoverErr.ClientMessage)
}

func TestOpenAIInvalidPromptPolicyErrorDoesNotChangeAccountState(t *testing.T) {
	account := &Account{
		ID:          31,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Credentials: map[string]any{"auth_mode": OpenAIAuthModeAgentIdentity},
	}
	repo := &accountTestAgentIdentityRepo{account: account}
	svc := &OpenAIGatewayService{accountRepo: repo}

	disabled := svc.handleOpenAIAccountUpstreamError(
		context.Background(),
		account,
		http.StatusBadGateway,
		http.Header{},
		[]byte(testOpenAIInvalidPromptPolicyBody),
	)

	require.False(t, disabled)
	require.Zero(t, repo.setErrorCalls)
	require.Equal(t, StatusActive, account.Status)
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
}

func TestOpenAIInvalidPromptPolicyUpstream502StopsForwardWithoutDisablingAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	key, privateKey := newTestAgentIdentityKey(t)
	account := &Account{
		ID:          32,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"auth_mode":          OpenAIAuthModeAgentIdentity,
			"agent_runtime_id":   key.runtimeID,
			"agent_private_key":  privateKey,
			"task_id":            key.taskID,
			"chatgpt_account_id": "account-invalid-prompt",
		},
	}
	repo := &accountTestAgentIdentityRepo{account: account}
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusBadGateway,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewBufferString(testOpenAIInvalidPromptPolicyBody)),
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, accountRepo: repo, httpUpstream: upstream}
	body := []byte(`{"model":"gpt-5.4","instructions":"Reply OK","input":[],"stream":false}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))

	_, err := svc.Forward(context.Background(), c, account, body)

	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.True(t, failoverErr.IsOpenAIInvalidPromptPolicyError())
	require.False(t, failoverErr.ShouldRetryNextAccount())
	require.Zero(t, repo.setErrorCalls)
	require.Equal(t, StatusActive, account.Status)
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
}
