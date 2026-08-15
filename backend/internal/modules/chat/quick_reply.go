package chat

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"
)

var errQuickReplyRepositoryUnavailable = errors.New("quick reply repository is unavailable")

func (s *Service) ListQuickReplies(ctx context.Context, adminID int64) ([]QuickReply, error) {
	if s.quickReplyRepo == nil {
		return nil, errQuickReplyRepositoryUnavailable
	}
	return s.quickReplyRepo.ListByAdminID(ctx, adminID)
}

func (s *Service) CreateQuickReply(ctx context.Context, adminID int64, title, content string) (*QuickReply, error) {
	if s.quickReplyRepo == nil {
		return nil, errQuickReplyRepositoryUnavailable
	}
	title, content, err := normalizeQuickReply(title, content)
	if err != nil {
		return nil, err
	}
	// The repository performs the count while holding the per-admin
	// transaction lock. A separate service-layer count would add a query and
	// still be stale by the time a concurrent create commits.
	reply := &QuickReply{AdminID: adminID, Title: title, Content: content}
	if err := s.quickReplyRepo.Create(ctx, reply); err != nil {
		return nil, err
	}
	return reply, nil
}

func (s *Service) UpdateQuickReply(ctx context.Context, adminID, id int64, title, content string) (*QuickReply, error) {
	if s.quickReplyRepo == nil {
		return nil, errQuickReplyRepositoryUnavailable
	}
	title, content, err := normalizeQuickReply(title, content)
	if err != nil {
		return nil, err
	}
	reply, err := s.quickReplyRepo.GetByID(ctx, adminID, id)
	if err != nil {
		return nil, err
	}
	reply.Title = title
	reply.Content = content
	if err := s.quickReplyRepo.Update(ctx, reply); err != nil {
		return nil, err
	}
	return reply, nil
}

func (s *Service) DeleteQuickReply(ctx context.Context, adminID, id int64) error {
	if s.quickReplyRepo == nil {
		return errQuickReplyRepositoryUnavailable
	}
	return s.quickReplyRepo.Delete(ctx, adminID, id)
}

func (s *Service) ReorderQuickReplies(ctx context.Context, adminID int64, orderedIDs []int64) error {
	if s.quickReplyRepo == nil {
		return errQuickReplyRepositoryUnavailable
	}
	if len(orderedIDs) > MaxQuickReplies || len(uniquePositiveIDs(orderedIDs)) != len(orderedIDs) {
		return ErrQuickReplyNotFound
	}
	return s.quickReplyRepo.Reorder(ctx, adminID, orderedIDs)
}

func (s *Service) ImportQuickReplies(ctx context.Context, adminID int64, inputs []QuickReply) ([]QuickReply, error) {
	if s.quickReplyRepo == nil {
		return nil, errQuickReplyRepositoryUnavailable
	}
	if len(inputs) > MaxQuickReplies {
		return nil, ErrQuickReplyLimitReached
	}
	cleaned := make([]QuickReply, 0, len(inputs))
	seen := make(map[string]struct{}, len(inputs))
	for _, input := range inputs {
		title, content, err := normalizeQuickReply(input.Title, input.Content)
		if err != nil {
			return nil, err
		}
		key := strings.ToLower(title) + "\x00" + content
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		cleaned = append(cleaned, QuickReply{AdminID: adminID, Title: title, Content: content})
	}
	return s.quickReplyRepo.Import(ctx, adminID, cleaned)
}

func normalizeQuickReply(title, content string) (string, string, error) {
	title = strings.TrimSpace(title)
	content = strings.TrimSpace(content)
	if title == "" || content == "" {
		return "", "", ErrMessageContentEmpty
	}
	if utf8.RuneCountInString(title) > 100 || utf8.RuneCountInString(content) > MaxMessageContentLen {
		return "", "", ErrMessageContentTooLong
	}
	return title, content, nil
}
