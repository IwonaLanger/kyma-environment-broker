package dbmodel

import (
	"encoding/json"
	"time"

	"github.com/kyma-project/kyma-environment-broker/internal"

	"github.com/kyma-project/kyma-environment-broker/common/orchestration"
)

// OrchestrationFilter holds the filters when listing orchestrations
type OrchestrationFilter struct {
	Page     int
	PageSize int
	Types    []string
	States   []string
}

type OrchestrationDTO struct {
	OrchestrationID string
	Type            string
	State           string
	Description     string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	Parameters      string
}

func NewOrchestrationDTO(o internal.Orchestration) (OrchestrationDTO, error) {
	params, err := json.Marshal(o.Parameters)
	if err != nil {
		return OrchestrationDTO{}, err
	}

	dto := OrchestrationDTO{
		OrchestrationID: o.OrchestrationID,
		Type:            string(o.Type),
		State:           o.State,
		CreatedAt:       o.CreatedAt,
		UpdatedAt:       o.UpdatedAt,
		Description:     o.Description,
		Parameters:      string(params),
	}
	return dto, nil
}

func (o *OrchestrationDTO) ToOrchestration() (internal.Orchestration, error) {
	var params orchestration.Parameters
	err := json.Unmarshal([]byte(o.Parameters), &params)
	if err != nil {
		return internal.Orchestration{}, err
	}
	return internal.Orchestration{
		OrchestrationID: o.OrchestrationID,
		Type:            orchestration.Type(o.Type),
		State:           o.State,
		Description:     o.Description,
		CreatedAt:       o.CreatedAt,
		UpdatedAt:       o.UpdatedAt,
		Parameters:      params,
	}, nil
}
