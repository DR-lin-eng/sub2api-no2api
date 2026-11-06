package service

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/shared/ctxkey"
)

func (s *OpenAIGatewayService) openAIPrivacyGroup(ctx context.Context, groupID *int64) *Group {
	if s == nil || groupID == nil {
		return nil
	}
	if selected := selectedAPIKeyRoutingGroup(ctx, *groupID); IsGroupContextValid(selected) {
		return selected
	}
	if group, _ := ctx.Value(ctxkey.Group).(*Group); IsGroupContextValid(group) && group.ID == *groupID {
		return group
	}
	if s.schedulerSnapshot == nil {
		return nil
	}
	group, _ := s.schedulerSnapshot.GetGroupByIDLite(ctx, *groupID)
	return group
}

type openAIPrivacyRequiredKey struct{}

func (s *OpenAIGatewayService) withOpenAIPrivacyRequirement(ctx context.Context, groupID *int64) context.Context {
	required := false
	if group := s.openAIPrivacyGroup(ctx, groupID); group != nil {
		required = group.RequirePrivacySet
	}
	return context.WithValue(ctx, openAIPrivacyRequiredKey{}, required)
}
func openAIPrivacyAllows(ctx context.Context, account *Account) bool {
	if ctx == nil {
		return true
	}
	required, _ := ctx.Value(openAIPrivacyRequiredKey{}).(bool)
	return !required || account.IsPrivacySet()
}
