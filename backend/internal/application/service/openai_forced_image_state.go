package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"
)

const openAIForcedImageResponseStateTTL = 30 * time.Minute

type openAIForcedImageResponseState struct {
	ResponseID string                           `json:"response_id"`
	Items      []OpenAIResponsesImageOutputItem `json:"items"`
}

func openAIForcedImageResponseStateKey(responseID string) string {
	return "forced-image-response:v1:" + strings.TrimSpace(responseID)
}

func openAIForcedImageItemStateKey(itemID string) string {
	return "forced-image-item:v1:" + strings.TrimSpace(itemID)
}

// StoreOpenAIForcedImageResponseState persists enough image output state to
// translate later previous_response_id and image_generation_call references
// into an Images API edit request. The WS shared-state store uses Redis when
// configured and its bounded in-process fallback otherwise.
func (s *OpenAIGatewayService) StoreOpenAIForcedImageResponseState(
	ctx context.Context,
	groupID int64,
	responseID string,
	items []OpenAIResponsesImageOutputItem,
) error {
	responseID = strings.TrimSpace(responseID)
	if s == nil || responseID == "" || len(items) == 0 {
		return nil
	}
	stateItems := make([]OpenAIResponsesImageOutputItem, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.ID) == "" || strings.TrimSpace(item.Result) == "" || item.Result == "discarded" {
			continue
		}
		stateItems = append(stateItems, item)
	}
	if len(stateItems) == 0 {
		return nil
	}
	encoded, err := json.Marshal(openAIForcedImageResponseState{ResponseID: responseID, Items: stateItems})
	if err != nil {
		return fmt.Errorf("encode forced image response state: %w", err)
	}
	store := s.getOpenAIWSStateStore()
	if store == nil {
		return fmt.Errorf("forced image response state store is unavailable")
	}
	storeCtx := context.WithoutCancel(ctx)
	if err := store.BindSessionTurnState(
		storeCtx,
		groupID,
		openAIForcedImageResponseStateKey(responseID),
		string(encoded),
		openAIForcedImageResponseStateTTL,
	); err != nil {
		return fmt.Errorf("store forced image response state: %w", err)
	}

	group, itemCtx := errgroup.WithContext(storeCtx)
	group.SetLimit(generatedImageDownloadConcurrency)
	for i := range stateItems {
		itemID := stateItems[i].ID
		group.Go(func() error {
			if err := store.BindSessionTurnState(
				itemCtx,
				groupID,
				openAIForcedImageItemStateKey(itemID),
				responseID,
				openAIForcedImageResponseStateTTL,
			); err != nil {
				return fmt.Errorf("store forced image item state: %w", err)
			}
			return nil
		})
	}
	return group.Wait()
}

// HydrateOpenAIResponsesImagePlan resolves official Responses continuation
// references into image inputs before the plan is materialized once and fanned
// out to independently scheduled n=1 Images API children.
func (s *OpenAIGatewayService) HydrateOpenAIResponsesImagePlan(
	ctx context.Context,
	groupID int64,
	plan *OpenAIResponsesImagePlan,
) error {
	if plan == nil {
		return fmt.Errorf("responses image plan is required")
	}
	if plan.forceGenerate || len(plan.inputs) > 0 {
		return nil
	}
	previousResponseID := strings.TrimSpace(plan.PreviousResponseID)
	if previousResponseID == "" && len(plan.inputItemIDs) == 0 {
		return nil
	}
	store := s.getOpenAIWSStateStore()
	if store == nil {
		return fmt.Errorf("forced image response state store is unavailable")
	}

	states := make(map[string]openAIForcedImageResponseState)
	loadResponse := func(responseID string) (openAIForcedImageResponseState, error) {
		responseID = strings.TrimSpace(responseID)
		if state, exists := states[responseID]; exists {
			return state, nil
		}
		encoded, found, err := store.GetSessionTurnState(ctx, groupID, openAIForcedImageResponseStateKey(responseID))
		if err != nil {
			return openAIForcedImageResponseState{}, fmt.Errorf("load previous image response: %w", err)
		}
		if !found {
			return openAIForcedImageResponseState{}, fmt.Errorf("previous image response %s was not found or has expired", responseID)
		}
		var state openAIForcedImageResponseState
		if err := json.Unmarshal([]byte(encoded), &state); err != nil {
			return openAIForcedImageResponseState{}, fmt.Errorf("decode previous image response: %w", err)
		}
		if len(state.Items) == 0 {
			return openAIForcedImageResponseState{}, fmt.Errorf("previous image response %s has no image output", responseID)
		}
		states[responseID] = state
		return state, nil
	}

	resolvedItems := make([]OpenAIResponsesImageOutputItem, 0, 1+len(plan.inputItemIDs))
	if previousResponseID != "" {
		state, err := loadResponse(previousResponseID)
		if err != nil {
			return err
		}
		resolvedItems = append(resolvedItems, state.Items...)
	}
	for _, itemID := range plan.inputItemIDs {
		responseID, found, err := store.GetSessionTurnState(ctx, groupID, openAIForcedImageItemStateKey(itemID))
		if err != nil {
			return fmt.Errorf("load previous image item: %w", err)
		}
		if !found {
			return fmt.Errorf("image_generation_call %s was not found or has expired", itemID)
		}
		state, err := loadResponse(responseID)
		if err != nil {
			return err
		}
		matched := false
		for _, item := range state.Items {
			if item.ID == itemID {
				resolvedItems = append(resolvedItems, item)
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("image_generation_call %s has no stored image output", itemID)
		}
	}

	seen := make(map[string]struct{}, len(resolvedItems))
	for _, item := range resolvedItems {
		result := strings.TrimSpace(item.Result)
		if result == "" || result == "discarded" {
			continue
		}
		if _, exists := seen[result]; exists {
			continue
		}
		seen[result] = struct{}{}
		imageURL := result
		lower := strings.ToLower(result)
		if !strings.HasPrefix(lower, "data:") && !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
			imageURL = "data:" + openAIForcedImageOutputMIME(item.OutputFormat) + ";base64," + result
		}
		plan.inputs = append(plan.inputs, openAIResponsesImageInput{ImageURL: imageURL})
	}
	if len(plan.inputs) == 0 {
		return fmt.Errorf("previous image response did not contain a reusable image")
	}
	plan.action = "edit"
	return nil
}

func openAIForcedImageOutputMIME(outputFormat string) string {
	switch strings.ToLower(strings.TrimSpace(outputFormat)) {
	case "jpg", "jpeg":
		return "image/jpeg"
	case "webp":
		return "image/webp"
	case "gif":
		return "image/gif"
	case "avif":
		return "image/avif"
	default:
		return "image/png"
	}
}
