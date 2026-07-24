package research

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/researchscreen"
)

const QuerySchemaVersion = broker.ScreenQuerySchemaVersionV2

// ScreenPreset is an instance-owned reusable stock-screen definition.
// Pagination and query results deliberately do not belong to the preset.
type ScreenPreset struct {
	ID                 string                    `json:"presetId"`
	Name               string                    `json:"name"`
	QuerySchemaVersion int                       `json:"querySchemaVersion"`
	Definition         broker.ScreenDefinitionV2 `json:"definition"`
	Revision           int64                     `json:"revision"`
	CreatedAt          time.Time                 `json:"createdAt"`
	UpdatedAt          time.Time                 `json:"updatedAt"`
}

type CreateScreenPresetInput struct {
	Name       string                    `json:"name" binding:"required"`
	Definition broker.ScreenDefinitionV2 `json:"definition" binding:"required"`
}

type UpdateScreenPresetInput struct {
	Name             *string                    `json:"name,omitempty"`
	Definition       *broker.ScreenDefinitionV2 `json:"definition,omitempty"`
	ExpectedRevision int64                      `json:"expectedRevision" binding:"required,min=1"`
}

type ScreenPresetRepository interface {
	ListScreenPresets(context.Context) ([]ScreenPreset, error)
	GetScreenPreset(context.Context, string) (ScreenPreset, error)
	CreateScreenPreset(context.Context, string, broker.ScreenDefinitionV2, int) (ScreenPreset, error)
	UpdateScreenPreset(context.Context, string, string, broker.ScreenDefinitionV2, int, int64) (ScreenPreset, error)
	DeleteScreenPreset(context.Context, string) error
}

type Service struct {
	repository ScreenPresetRepository
}

func NewService(repository ScreenPresetRepository) *Service {
	return &Service{repository: repository}
}

func (s *Service) ListScreenPresets(ctx context.Context) ([]ScreenPreset, error) {
	if s == nil || s.repository == nil {
		return nil, ErrUnavailable
	}
	return s.repository.ListScreenPresets(ctx)
}

func (s *Service) GetScreenPreset(ctx context.Context, presetID string) (ScreenPreset, error) {
	if s == nil || s.repository == nil {
		return ScreenPreset{}, ErrUnavailable
	}
	if strings.TrimSpace(presetID) == "" {
		return ScreenPreset{}, fmt.Errorf("%w: preset id is required", ErrValidation)
	}
	return s.repository.GetScreenPreset(ctx, strings.TrimSpace(presetID))
}

func (s *Service) CreateScreenPreset(ctx context.Context, input CreateScreenPresetInput) (ScreenPreset, error) {
	if s == nil || s.repository == nil {
		return ScreenPreset{}, ErrUnavailable
	}
	name, err := normalizePresetName(input.Name)
	if err != nil {
		return ScreenPreset{}, err
	}
	definition, err := normalizePresetDefinition(input.Definition)
	if err != nil {
		return ScreenPreset{}, err
	}
	return s.repository.CreateScreenPreset(ctx, name, definition, QuerySchemaVersion)
}

func (s *Service) UpdateScreenPreset(ctx context.Context, presetID string, input UpdateScreenPresetInput) (ScreenPreset, error) {
	if s == nil || s.repository == nil {
		return ScreenPreset{}, ErrUnavailable
	}
	presetID = strings.TrimSpace(presetID)
	if presetID == "" {
		return ScreenPreset{}, fmt.Errorf("%w: preset id is required", ErrValidation)
	}
	if input.ExpectedRevision < 1 {
		return ScreenPreset{}, fmt.Errorf("%w: expectedRevision must be positive", ErrValidation)
	}
	if input.Name == nil && input.Definition == nil {
		return ScreenPreset{}, fmt.Errorf("%w: name or definition is required", ErrValidation)
	}
	current, err := s.repository.GetScreenPreset(ctx, presetID)
	if err != nil {
		return ScreenPreset{}, err
	}
	if current.Revision != input.ExpectedRevision {
		return ScreenPreset{}, ErrConflict
	}
	name := current.Name
	if input.Name != nil {
		name, err = normalizePresetName(*input.Name)
		if err != nil {
			return ScreenPreset{}, err
		}
	}
	definition := current.Definition
	if input.Definition != nil {
		definition, err = normalizePresetDefinition(*input.Definition)
		if err != nil {
			return ScreenPreset{}, err
		}
	}
	return s.repository.UpdateScreenPreset(
		ctx, presetID, name, definition, QuerySchemaVersion, input.ExpectedRevision,
	)
}

func (s *Service) DeleteScreenPreset(ctx context.Context, presetID string) error {
	if s == nil || s.repository == nil {
		return ErrUnavailable
	}
	presetID = strings.TrimSpace(presetID)
	if presetID == "" {
		return fmt.Errorf("%w: preset id is required", ErrValidation)
	}
	return s.repository.DeleteScreenPreset(ctx, presetID)
}

func normalizePresetName(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("%w: name is required", ErrValidation)
	}
	if len([]rune(value)) > 80 {
		return "", fmt.Errorf("%w: name must not exceed 80 characters", ErrValidation)
	}
	return value, nil
}

func normalizePresetDefinition(definition broker.ScreenDefinitionV2) (broker.ScreenDefinitionV2, error) {
	normalized, err := researchscreen.NormalizeDefinitionV2(definition)
	if err != nil {
		return broker.ScreenDefinitionV2{}, fmt.Errorf("%w: %v", ErrValidation, err)
	}
	return normalized, nil
}
