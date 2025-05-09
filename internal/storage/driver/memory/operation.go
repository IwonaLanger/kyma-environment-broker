package memory

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/kyma-project/kyma-environment-broker/common/pagination"
	"github.com/kyma-project/kyma-environment-broker/internal"
	"github.com/kyma-project/kyma-environment-broker/internal/storage/dberr"
	"github.com/kyma-project/kyma-environment-broker/internal/storage/dbmodel"

	"github.com/pivotal-cf/brokerapi/v12/domain"
)

type operations struct {
	mu sync.Mutex

	operations               map[string]internal.Operation
	upgradeClusterOperations map[string]internal.UpgradeClusterOperation
	updateOperations         map[string]internal.UpdatingOperation
}

// NewOperation creates in-memory storage for OSB operations.
func NewOperation() *operations {
	return &operations{
		operations:               make(map[string]internal.Operation, 0),
		upgradeClusterOperations: make(map[string]internal.UpgradeClusterOperation, 0),
		updateOperations:         make(map[string]internal.UpdatingOperation, 0),
	}
}

func (s *operations) DeleteByID(operationID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.operations, operationID)
	return nil
}

func (s *operations) InsertProvisioningOperation(operation internal.ProvisioningOperation) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := operation.ID
	if _, exists := s.operations[id]; exists {
		return dberr.AlreadyExists("instance operation with id %s already exist", id)
	}

	s.operations[id] = operation.Operation
	return nil
}

func (s *operations) InsertOperation(operation internal.Operation) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := operation.ID
	if _, exists := s.operations[id]; exists {
		return dberr.AlreadyExists("instance operation with id %s already exist", id)
	}

	s.operations[id] = operation
	return nil
}

func (s *operations) GetProvisioningOperationByID(operationID string) (*internal.ProvisioningOperation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	op, exists := s.operations[operationID]
	if !exists {
		return nil, dberr.NotFound("instance provisioning operation with id %s not found", operationID)
	}
	return &internal.ProvisioningOperation{Operation: op}, nil
}

func (s *operations) GetProvisioningOperationByInstanceID(instanceID string) (*internal.ProvisioningOperation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var result []internal.ProvisioningOperation

	for _, op := range s.operations {
		if op.InstanceID == instanceID && op.Type == internal.OperationTypeProvision {
			result = append(result, internal.ProvisioningOperation{Operation: op})
		}
	}
	if len(result) != 0 {
		s.sortProvisioningByCreatedAtDesc(result)
		return &result[0], nil
	}

	return nil, dberr.NotFound("instance provisioning operation with instanceID %s not found", instanceID)
}

func (s *operations) GetOperationByInstanceID(instanceID string) (*internal.Operation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var result []internal.Operation

	for _, op := range s.operations {
		if op.InstanceID == instanceID {
			result = append(result, op)
		}
	}
	if len(result) != 0 {
		s.sortOperationsByCreatedAtDesc(result)
		return &result[0], nil
	}

	return nil, dberr.NotFound("instance provisioning operation with instanceID %s not found", instanceID)
}

func (s *operations) UpdateProvisioningOperation(op internal.ProvisioningOperation) (*internal.ProvisioningOperation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	oldOp, exists := s.operations[op.ID]
	if !exists {
		return nil, dberr.NotFound("instance operation with id %s not found", op.ID)
	}
	if oldOp.Version != op.Version {
		return nil, dberr.Conflict("unable to update provisioning operation with id %s (for instance id %s) - conflict", op.ID, op.InstanceID)
	}
	op.Version = op.Version + 1
	s.operations[op.ID] = op.Operation

	return &op, nil
}

func (s *operations) UpdateOperation(op internal.Operation) (*internal.Operation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	oldOp, exists := s.operations[op.ID]
	if !exists {
		return nil, dberr.NotFound("instance operation with id %s not found", op.ID)
	}
	if oldOp.Version != op.Version {
		return nil, dberr.Conflict("unable to update operation with id %s (for instance id %s) - conflict", op.ID, op.InstanceID)
	}
	op.Version = op.Version + 1
	s.operations[op.ID] = op

	return &op, nil
}

func (s *operations) ListProvisioningOperationsByInstanceID(instanceID string) ([]internal.ProvisioningOperation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	operations := make([]internal.ProvisioningOperation, 0)
	for _, op := range s.operations {
		if op.InstanceID == instanceID && op.Type == internal.OperationTypeProvision {
			operations = append(operations, internal.ProvisioningOperation{Operation: op})
		}
	}

	if len(operations) != 0 {
		s.sortProvisioningByCreatedAtDesc(operations)
		return operations, nil
	}

	return nil, dberr.NotFound("instance provisioning operations with instanceID %s not found", instanceID)
}

func (s *operations) ListOperationsByInstanceID(instanceID string) ([]internal.Operation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	operations := make([]internal.Operation, 0)
	for _, op := range s.operations {
		if op.InstanceID == instanceID {
			operations = append(operations, op)
		}
	}

	s.sortOperationsByCreatedAtDesc(operations)
	return operations, nil
}

func (s *operations) ListOperationsByInstanceIDGroupByType(instanceID string) (*internal.GroupedOperations, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	grouped := internal.GroupedOperations{
		ProvisionOperations:      make([]internal.ProvisioningOperation, 0),
		DeprovisionOperations:    make([]internal.DeprovisioningOperation, 0),
		UpgradeClusterOperations: make([]internal.UpgradeClusterOperation, 0),
		UpdateOperations:         make([]internal.UpdatingOperation, 0),
	}

	for _, op := range s.operations {
		switch op.Type {
		case internal.OperationTypeProvision:
			grouped.ProvisionOperations = append(grouped.ProvisionOperations, internal.ProvisioningOperation{Operation: op})

		case internal.OperationTypeDeprovision:
			grouped.DeprovisionOperations = append(grouped.DeprovisionOperations, internal.DeprovisioningOperation{Operation: op})

		case internal.OperationTypeUpgradeCluster:
			grouped.UpgradeClusterOperations = append(grouped.UpgradeClusterOperations, internal.UpgradeClusterOperation{Operation: op})

		case internal.OperationTypeUpdate:
			grouped.UpdateOperations = append(grouped.UpdateOperations, internal.UpdatingOperation{Operation: op})
		default:
			panic("Invalid type of operation")
		}
	}

	s.sortProvisioningByCreatedAtDesc(grouped.ProvisionOperations)
	s.sortDeprovisioningByCreatedAtDesc(grouped.DeprovisionOperations)
	s.sortUpgradeClusterByCreatedAt(grouped.UpgradeClusterOperations)
	s.sortUpdateByCreatedAt(grouped.UpdateOperations)

	return &grouped, nil
}

func (s *operations) ListOperationsInTimeRange(from, to time.Time) ([]internal.Operation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	operations := make([]internal.Operation, 0)
	for _, op := range s.operations {
		if (op.CreatedAt.After(from) || op.CreatedAt == from) && (op.CreatedAt.Before(to) || op.CreatedAt == to) {
			operations = append(operations, op)
			continue
		}

		if (op.UpdatedAt.After(from) || op.UpdatedAt == from) && (op.UpdatedAt.Before(to) || op.UpdatedAt == to) {
			operations = append(operations, op)
			continue
		}
	}

	return operations, nil
}

func (s *operations) InsertDeprovisioningOperation(operation internal.DeprovisioningOperation) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := operation.ID
	if _, exists := s.operations[id]; exists {
		return dberr.AlreadyExists("instance operation with id %s already exist", id)
	}

	s.operations[id] = operation.Operation
	return nil
}

func (s *operations) GetDeprovisioningOperationByID(operationID string) (*internal.DeprovisioningOperation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	op, exists := s.operations[operationID]
	if !exists {
		return nil, dberr.NotFound("instance deprovisioning operation with id %s not found", operationID)
	}
	return &internal.DeprovisioningOperation{Operation: op}, nil
}

func (s *operations) GetDeprovisioningOperationByInstanceID(instanceID string) (*internal.DeprovisioningOperation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var result []internal.DeprovisioningOperation

	for _, op := range s.operations {
		if op.InstanceID == instanceID && op.Type == internal.OperationTypeDeprovision {
			result = append(result, internal.DeprovisioningOperation{Operation: op})
		}
	}
	if len(result) != 0 {
		s.sortDeprovisioningByCreatedAtDesc(result)
		return &result[0], nil
	}

	return nil, dberr.NotFound("instance deprovisioning operation with instanceID %s not found", instanceID)
}

func (s *operations) UpdateDeprovisioningOperation(op internal.DeprovisioningOperation) (*internal.DeprovisioningOperation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	oldOp, exists := s.operations[op.ID]
	if !exists {
		return nil, dberr.NotFound("instance operation with id %s not found", op.ID)
	}
	if oldOp.Version != op.Version {
		return nil, dberr.Conflict("unable to update deprovisioning operation with id %s (for instance id %s) - conflict", op.ID, op.InstanceID)
	}
	op.Version = op.Version + 1
	s.operations[op.ID] = op.Operation

	return &op, nil
}

func (s *operations) ListDeprovisioningOperationsByInstanceID(instanceID string) ([]internal.DeprovisioningOperation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	operations := make([]internal.DeprovisioningOperation, 0)
	for _, op := range s.operations {
		if op.InstanceID == instanceID && op.Type == internal.OperationTypeDeprovision {
			operations = append(operations, internal.DeprovisioningOperation{Operation: op})
		}
	}

	if len(operations) != 0 {
		s.sortDeprovisioningByCreatedAtDesc(operations)
		return operations, nil
	}

	return nil, dberr.NotFound("instance deprovisioning operations with instanceID %s not found", instanceID)
}

func (s *operations) ListDeprovisioningOperations() ([]internal.DeprovisioningOperation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	operations := make([]internal.DeprovisioningOperation, 0)
	for _, op := range s.operations {
		if op.Type == internal.OperationTypeDeprovision {
			operations = append(operations, internal.DeprovisioningOperation{Operation: op})
		}
	}

	s.sortDeprovisioningByCreatedAtDesc(operations)
	return operations, nil
}

func (s *operations) InsertUpgradeClusterOperation(operation internal.UpgradeClusterOperation) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := operation.Operation.ID
	if _, exists := s.upgradeClusterOperations[id]; exists {
		return dberr.AlreadyExists("instance operation with id %s already exist", id)
	}

	s.upgradeClusterOperations[id] = operation
	return nil
}

func (s *operations) GetUpgradeClusterOperationByID(operationID string) (*internal.UpgradeClusterOperation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	op, exists := s.upgradeClusterOperations[operationID]
	if !exists {
		return nil, dberr.NotFound("instance upgradeCluster operation with id %s not found", operationID)
	}
	return &op, nil
}

func (s *operations) UpdateUpgradeClusterOperation(op internal.UpgradeClusterOperation) (*internal.UpgradeClusterOperation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	oldOp, exists := s.upgradeClusterOperations[op.Operation.ID]
	if !exists {
		return nil, dberr.NotFound("instance operation with id %s not found", op.Operation.ID)
	}
	if oldOp.Version != op.Version {
		return nil, dberr.Conflict("unable to update upgradeKyma operation with id %s (for instance id %s) - conflict", op.Operation.ID, op.InstanceID)
	}
	op.Version = op.Version + 1
	s.upgradeClusterOperations[op.Operation.ID] = op

	return &op, nil
}

func (s *operations) GetLastOperationByTypes(instanceID string, types []internal.OperationType) (*internal.Operation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var rows []internal.Operation

	for _, op := range s.operations {
		if op.InstanceID == instanceID && op.State != internal.OperationStatePending {
			if len(types) > 0 {
				for _, t := range types {
					if op.Type == t {
						rows = append(rows, op)
					}
				}
			} else {
				rows = append(rows, op)
			}
		}
	}
	for _, op := range s.upgradeClusterOperations {
		if op.InstanceID == instanceID && op.State != internal.OperationStatePending {
			if len(types) > 0 {
				for _, t := range types {
					if op.Type == t {
						rows = append(rows, op.Operation)
					}
				}
			} else {
				rows = append(rows, op.Operation)
			}
		}
	}
	for _, op := range s.updateOperations {
		if op.InstanceID == instanceID && op.State != internal.OperationStatePending {
			if len(types) > 0 {
				for _, t := range types {
					if op.Type == t {
						rows = append(rows, op.Operation)
					}
				}
			} else {
				rows = append(rows, op.Operation)
			}
		}
	}

	if len(rows) == 0 {
		return nil, dberr.NotFound("Operation with instance_id %s not exist", instanceID)
	}

	sort.Slice(rows, func(i, j int) bool {
		return rows[i].CreatedAt.After(rows[j].CreatedAt)
	})

	return &rows[0], nil
}

func (s *operations) GetLastOperation(instanceID string) (*internal.Operation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var rows []internal.Operation

	for _, op := range s.operations {
		if op.InstanceID == instanceID && op.State != internal.OperationStatePending {
			rows = append(rows, op)
		}
	}
	for _, op := range s.upgradeClusterOperations {
		if op.InstanceID == instanceID && op.State != internal.OperationStatePending {
			rows = append(rows, op.Operation)
		}
	}
	for _, op := range s.updateOperations {
		if op.InstanceID == instanceID && op.State != internal.OperationStatePending {
			rows = append(rows, op.Operation)
		}
	}

	if len(rows) == 0 {
		return nil, dberr.NotFound("Operation with instance_id %s not exist", instanceID)
	}

	sort.Slice(rows, func(i, j int) bool {
		return rows[i].CreatedAt.After(rows[j].CreatedAt)
	})

	return &rows[0], nil
}

func (s *operations) GetOperationByID(operationID string) (*internal.Operation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var res *internal.Operation

	provisionOp, exists := s.operations[operationID]
	if exists {
		res = &provisionOp
	}
	upgradeClusterOp, exists := s.upgradeClusterOperations[operationID]
	if exists {
		res = &upgradeClusterOp.Operation
	}
	updateOp, exists := s.updateOperations[operationID]
	if exists {
		res = &updateOp.Operation
	}
	op, exists := s.operations[operationID]
	if exists {
		res = &op
	}

	if res == nil {
		return nil, dberr.NotFound("Operation with id %s not exist", operationID)
	}

	return res, nil
}

func (s *operations) GetNotFinishedOperationsByType(opType internal.OperationType) ([]internal.Operation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ops := make([]internal.Operation, 0)
	switch opType {
	case internal.OperationTypeProvision:
		for _, op := range s.operations {
			if op.State == domain.InProgress {
				ops = append(ops, op)
			}
		}
	case internal.OperationTypeDeprovision:
		for _, op := range s.operations {
			if op.State == domain.InProgress {
				ops = append(ops, op)
			}
		}
	}

	return ops, nil
}

func (s *operations) GetOperationsForIDs(opIdList []string) ([]internal.Operation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ops := make([]internal.Operation, 0)
	for _, opID := range opIdList {
		for _, op := range s.updateOperations {
			if op.Operation.ID == opID {
				ops = append(ops, op.Operation)
			}
		}
	}
	for _, opID := range opIdList {
		for _, op := range s.upgradeClusterOperations {
			if op.Operation.ID == opID {
				ops = append(ops, op.Operation)
			}
		}
	}

	for _, opID := range opIdList {
		for _, op := range s.operations {
			if op.ID == opID {
				ops = append(ops, op)
			}
		}
	}

	if len(ops) == 0 {
		return nil, dberr.NotFound("operations with ids from list %+q not exist", opIdList)
	}

	return ops, nil
}

func (s *operations) GetOperationStatsByPlan() (map[string]internal.OperationStats, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := make(map[string]internal.OperationStats)

	for _, op := range s.operations {
		if op.ProvisioningParameters.PlanID == "" {
			continue
		}
		if _, ok := result[op.ProvisioningParameters.PlanID]; !ok {
			result[op.ProvisioningParameters.PlanID] = internal.OperationStats{
				Provisioning:   make(map[domain.LastOperationState]int),
				Deprovisioning: make(map[domain.LastOperationState]int),
			}
		}
		switch op.Type {
		case internal.OperationTypeProvision:
			result[op.ProvisioningParameters.PlanID].Provisioning[op.State] += 1
		case internal.OperationTypeDeprovision:
			result[op.ProvisioningParameters.PlanID].Deprovisioning[op.State] += 1
		}
	}
	return result, nil
}

func (s *operations) GetOperationStatsByPlanV2() ([]internal.OperationStatsV2, error) {
	stats := make([]internal.OperationStatsV2, 0)
	exists := func(item internal.OperationStatsV2) int {
		for idx, state := range stats {
			if state.State == item.State && state.Type == item.Type && state.PlanID == item.PlanID {
				return idx
			}
		}
		return -1
	}

	for _, op := range s.operations {
		if op.State == domain.InProgress {
			o := internal.OperationStatsV2{
				PlanID: op.ProvisioningParameters.PlanID,
				Type:   op.Type,
				State:  op.State,
			}

			if idx := exists(o); idx >= 0 {
				stats[idx].Count++
			} else {
				o.Count = 1
				stats = append(stats, o)
			}
		}
	}

	return stats, nil
}

func (s *operations) ListOperations(filter dbmodel.OperationFilter) ([]internal.Operation, int, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := make([]internal.Operation, 0)
	offset := pagination.ConvertPageAndPageSizeToOffset(filter.PageSize, filter.Page)

	operations, err := s.filterAll(filter)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("while listing operations: %w", err)
	}
	s.sortByCreatedAt(operations)

	for i := offset; (filter.PageSize < 1 || i < offset+filter.PageSize) && i < len(operations)+offset; i++ {
		result = append(result, operations[i])
	}

	return result,
		len(result),
		len(operations),
		nil
}

func (s *operations) ListUpgradeClusterOperationsByInstanceID(instanceID string) ([]internal.UpgradeClusterOperation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Empty filter means get all
	operations := s.filterUpgradeClusterByInstanceID(instanceID, dbmodel.OperationFilter{})

	if len(operations) != 0 {
		s.sortUpgradeClusterByCreatedAtDesc(operations)
		return operations, nil
	}

	return nil, dberr.NotFound("instance upgrade operations with instanceID %s not found", instanceID)
}

func (s *operations) InsertUpdatingOperation(operation internal.UpdatingOperation) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := operation.ID
	if _, exists := s.updateOperations[id]; exists {
		return dberr.AlreadyExists("instance operation with id %s already exist", id)
	}

	s.updateOperations[id] = operation
	return nil
}

func (s *operations) ListShortOperationsByInstanceID(instanceID string) ([]internal.Operation, error) {
	panic("not implemented")
}

func (s *operations) GetUpdatingOperationByID(operationID string) (*internal.UpdatingOperation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, op := range s.updateOperations {
		if op.ID == operationID {
			return &op, nil
		}
	}

	return nil, dberr.NotFound("instance update operation with ID %s not found", operationID)
}

func (s *operations) UpdateUpdatingOperation(op internal.UpdatingOperation) (*internal.UpdatingOperation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	oldOp, exists := s.updateOperations[op.ID]
	if !exists {
		return nil, dberr.NotFound("instance operation with id %s not found", op.ID)
	}
	if oldOp.Version != op.Version {
		return nil, dberr.Conflict("unable to update updating operation with id %s (for instance id %s) - conflict", op.ID, op.InstanceID)
	}
	op.Version = op.Version + 1
	s.updateOperations[op.ID] = op

	return &op, nil
}

func (s *operations) ListUpdatingOperationsByInstanceID(instanceID string) ([]internal.UpdatingOperation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	operations := make([]internal.UpdatingOperation, 0)
	for _, v := range s.updateOperations {
		if instanceID != v.InstanceID {
			continue
		}

		operations = append(operations, v)
	}

	sort.Slice(operations, func(i, j int) bool {
		return operations[i].CreatedAt.Before(operations[j].CreatedAt)
	})

	return operations, nil
}

func (s *operations) sortUpgradeClusterByCreatedAt(operations []internal.UpgradeClusterOperation) {
	sort.Slice(operations, func(i, j int) bool {
		return operations[i].CreatedAt.Before(operations[j].CreatedAt)
	})
}

func (s *operations) sortUpgradeClusterByCreatedAtDesc(operations []internal.UpgradeClusterOperation) {
	sort.Slice(operations, func(i, j int) bool {
		return operations[i].CreatedAt.After(operations[j].CreatedAt)
	})
}

func (s *operations) sortUpdateByCreatedAt(operations []internal.UpdatingOperation) {
	sort.Slice(operations, func(i, j int) bool {
		return operations[i].CreatedAt.Before(operations[j].CreatedAt)
	})
}

func (s *operations) sortProvisioningByCreatedAtDesc(operations []internal.ProvisioningOperation) {
	sort.Slice(operations, func(i, j int) bool {
		return operations[i].CreatedAt.After(operations[j].CreatedAt)
	})
}

func (s *operations) sortOperationsByCreatedAtDesc(operations []internal.Operation) {
	sort.Slice(operations, func(i, j int) bool {
		return operations[i].CreatedAt.After(operations[j].CreatedAt)
	})
}

func (s *operations) sortDeprovisioningByCreatedAtDesc(operations []internal.DeprovisioningOperation) {
	sort.Slice(operations, func(i, j int) bool {
		return operations[i].CreatedAt.After(operations[j].CreatedAt)
	})
}

func (s *operations) sortByCreatedAt(operations []internal.Operation) {
	sort.Slice(operations, func(i, j int) bool {
		return operations[i].CreatedAt.Before(operations[j].CreatedAt)
	})
}

func (s *operations) getAll() ([]internal.Operation, error) {
	ops := make([]internal.Operation, 0)
	for _, op := range s.upgradeClusterOperations {
		ops = append(ops, op.Operation)
	}
	for _, op := range s.operations {
		ops = append(ops, op)
	}

	return ops, nil
}

func (s *operations) filterAll(filter dbmodel.OperationFilter) ([]internal.Operation, error) {
	result := make([]internal.Operation, 0)
	ops, err := s.getAll()
	if err != nil {
		return nil, err
	}
	for _, op := range ops {
		if ok := matchFilter(string(op.State), filter.States, s.equalFilter); !ok {
			continue
		}
		result = append(result, op)
	}
	return result, nil
}

func (s *operations) filterUpgradeClusterByInstanceID(instanceID string, filter dbmodel.OperationFilter) []internal.UpgradeClusterOperation {
	operations := make([]internal.UpgradeClusterOperation, 0)
	for _, v := range s.upgradeClusterOperations {
		if instanceID != "" && instanceID != v.InstanceID {
			continue
		}
		if ok := matchFilter(string(v.State), filter.States, s.equalFilter); !ok {
			continue
		}

		operations = append(operations, v)
	}

	return operations
}

func (s *operations) equalFilter(a, b string) bool {
	return a == b
}

func (s *operations) GetAllOperations() ([]internal.Operation, error) {
	ops := make([]internal.Operation, 0)
	for _, k := range s.operations {
		ops = append(ops, k)
	}
	return ops, nil
}
