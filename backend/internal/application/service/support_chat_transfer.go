package service

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/modules/chat"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/shared/errors"
)

const (
	maxSupportChatTransferAmount = 1_000_000_000
	maxSupportChatTransferNotes  = 500
)

var ErrSupportChatTransferInvalid = infraerrors.BadRequest(
	"SUPPORT_CHAT_TRANSFER_INVALID",
	"balance transfer request is invalid",
)

type SupportChatTransferResult struct {
	Message  *chat.Message `json:"message"`
	UserID   int64         `json:"user_id"`
	Balance  float64       `json:"balance"`
	Replayed bool          `json:"replayed,omitempty"`
}

type supportChatBalanceAdminService interface {
	UpdateUserBalance(ctx context.Context, userID int64, balance float64, operation string, notes string) (*User, error)
}

// SupportChatTransferService commits the balance credit and its visible chat
// receipt in one PostgreSQL transaction. The chat message idempotency key is
// the durable recovery marker if the client loses the response after commit.
type SupportChatTransferService struct {
	adminService supportChatBalanceAdminService
	chatService  *chat.Service
	entClient    *dbent.Client
}

func NewSupportChatTransferService(
	adminService AdminService,
	chatService *chat.Service,
	entClient *dbent.Client,
) *SupportChatTransferService {
	return &SupportChatTransferService{
		adminService: adminService,
		chatService:  chatService,
		entClient:    entClient,
	}
}

func (s *SupportChatTransferService) Transfer(
	ctx context.Context,
	conversationID, adminID int64,
	amount float64,
	notes, idempotencyKey string,
) (*SupportChatTransferResult, error) {
	if s == nil || s.adminService == nil || s.chatService == nil || s.entClient == nil ||
		conversationID <= 0 || adminID <= 0 || amount <= 0 || amount > maxSupportChatTransferAmount ||
		math.IsNaN(amount) || math.IsInf(amount, 0) {
		return nil, ErrSupportChatTransferInvalid
	}
	notes = strings.TrimSpace(notes)
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if utf8.RuneCountInString(notes) > maxSupportChatTransferNotes {
		return nil, ErrSupportChatTransferInvalid
	}

	conversation, err := s.chatService.GetConversationForAdmin(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	if recovered, recoverErr := s.recoverTransfer(ctx, conversation, adminID, amount, notes, idempotencyKey); recoverErr == nil {
		return recovered, nil
	} else if !errors.Is(recoverErr, chat.ErrMessageNotFound) {
		return nil, recoverErr
	}

	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin support chat balance transfer: %w", err)
	}
	txCtx := dbent.NewTxContext(ctx, tx)
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Client().ExecContext(
		txCtx,
		"SELECT pg_advisory_xact_lock($1)",
		supportChatTransferLockKey(adminID, idempotencyKey),
	); err != nil {
		return nil, fmt.Errorf("lock support chat balance transfer: %w", err)
	}

	// The outer request coordinator normally serializes the same key, but the
	// receipt is the durable source of truth. Re-check it after taking a
	// PostgreSQL transaction lock and before changing the balance so concurrent
	// calls cannot both credit the user when that coordinator is bypassed or a
	// response is retried after its lease expires.
	if recovered, recoverErr := s.recoverTransfer(txCtx, conversation, adminID, amount, notes, idempotencyKey); recoverErr == nil {
		_ = tx.Rollback()
		return recovered, nil
	} else if !errors.Is(recoverErr, chat.ErrMessageNotFound) {
		return nil, recoverErr
	}

	// Resolve the conversation again inside the transaction so deletion or
	// reassignment cannot race the credit and receipt creation.
	conversation, err = s.chatService.GetConversationForAdmin(txCtx, conversationID)
	if err != nil {
		return nil, err
	}
	user, err := s.adminService.UpdateUserBalance(txCtx, conversation.UserID, amount, "add", notes)
	if err != nil {
		return nil, err
	}
	message, err := s.chatService.PostBalanceTransferFromAdmin(
		txCtx,
		conversationID,
		adminID,
		chat.BalanceTransferMetadata{Amount: amount, BalanceAfter: user.Balance, Notes: notes},
		idempotencyKey,
		false,
	)
	if err != nil {
		// A concurrent request may have committed the same durable key while
		// this transaction waited on the user row. Roll back our credit before
		// attempting recovery outside the aborted transaction.
		_ = tx.Rollback()
		if recovered, recoverErr := s.recoverTransfer(ctx, conversation, adminID, amount, notes, idempotencyKey); recoverErr == nil {
			return recovered, nil
		}
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		if recovered, recoverErr := s.recoverTransfer(ctx, conversation, adminID, amount, notes, idempotencyKey); recoverErr == nil {
			return recovered, nil
		}
		return nil, fmt.Errorf("commit support chat balance transfer: %w", err)
	}

	s.chatService.BroadcastPersistedMessage(conversation.UserID, message)
	return &SupportChatTransferResult{
		Message: message,
		UserID:  conversation.UserID,
		Balance: user.Balance,
	}, nil
}

func supportChatTransferLockKey(adminID int64, idempotencyKey string) int64 {
	digest := sha256.Sum256([]byte(strconv.FormatInt(adminID, 10) + "\x00" + idempotencyKey))
	return int64(binary.BigEndian.Uint64(digest[:8]))
}

func (s *SupportChatTransferService) recoverTransfer(
	ctx context.Context,
	conversation *chat.Conversation,
	adminID int64,
	amount float64,
	notes, idempotencyKey string,
) (*SupportChatTransferResult, error) {
	message, err := s.chatService.GetMessageByIdempotencyKey(ctx, chat.SenderTypeAdmin, adminID, idempotencyKey)
	if err != nil {
		return nil, err
	}
	var metadata chat.BalanceTransferMetadata
	if conversation == nil || message.ConversationID != conversation.ID || message.Kind != chat.MessageKindBalanceTransfer ||
		json.Unmarshal(message.Metadata, &metadata) != nil || metadata.Amount != amount || metadata.Notes != notes {
		return nil, chat.ErrMessageIdempotencyConflict
	}
	return &SupportChatTransferResult{
		Message:  message,
		UserID:   conversation.UserID,
		Balance:  metadata.BalanceAfter,
		Replayed: true,
	}, nil
}
