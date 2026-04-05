package app

import (
	"errors"
	"fmt"
	"os"
)

type plannedTargetChanges struct {
	Name    string
	Changes []updateTargetChange
}

type targetChangePlanFailure struct {
	Name        string
	Err         error
	RollbackErr error
}

func (s Service) applyTargetChangePlan(
	plan plannedTargetChanges,
	applied []plannedTargetChanges,
	rollback func([]plannedTargetChanges) error,
) (plannedTargetChanges, *targetChangePlanFailure) {
	appliedCount := 0
	for i := range plan.Changes {
		backupPath, err := s.applyUpdateTargetChange(plan.Name, plan.Changes[i])
		if err != nil {
			rollbackPlans := append([]plannedTargetChanges(nil), applied...)
			if appliedCount > 0 {
				rollbackPlans = append(rollbackPlans, plannedTargetChanges{
					Name:    plan.Name,
					Changes: append([]updateTargetChange(nil), plan.Changes[:appliedCount]...),
				})
			}
			return plannedTargetChanges{}, &targetChangePlanFailure{
				Name:        plan.Name,
				Err:         err,
				RollbackErr: rollback(rollbackPlans),
			}
		}
		plan.Changes[i].BackupPath = backupPath
		appliedCount++
	}
	plan.Changes = plan.Changes[:appliedCount]
	return plan, nil
}

func (s Service) rollbackTargetChangePlans(applied []plannedTargetChanges) error {
	var rollbackErr error
	for i := len(applied) - 1; i >= 0; i-- {
		rollbackErr = errors.Join(rollbackErr, s.rollbackTargetChangePlan(applied[i]))
	}
	return rollbackErr
}

func (s Service) rollbackTargetChangePlan(plan plannedTargetChanges) error {
	var rollbackErr error
	for i := len(plan.Changes) - 1; i >= 0; i-- {
		change := plan.Changes[i]
		if change.PreviousPath == change.DesiredPath && change.BackupPath == "" {
			continue
		}
		if _, err := s.applyUpdateTargetChange(plan.Name, reverseUpdateTargetChange(change)); err != nil {
			rollbackErr = errors.Join(rollbackErr, err)
		}
	}
	return rollbackErr
}

func cleanupTargetChangePlanBackups(plans []plannedTargetChanges) {
	for _, plan := range plans {
		cleanupTargetChangeBackups(plan.Changes)
	}
}

func cleanupTargetChangeBackups(changes []updateTargetChange) {
	for _, change := range changes {
		if change.BackupPath != "" {
			os.RemoveAll(change.BackupPath)
		}
	}
}

func formatTargetChangeFailure(prefix string, failure *targetChangePlanFailure) error {
	if failure == nil {
		return nil
	}
	if failure.RollbackErr != nil {
		return fmt.Errorf("%s%w (rollback failed: %v)", prefix, failure.Err, failure.RollbackErr)
	}
	return fmt.Errorf("%s%w", prefix, failure.Err)
}
