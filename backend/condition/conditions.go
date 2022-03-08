package condition

import (
	"github.com/redhat-appstudio/managed-gitops/backend/apis/managed-gitops/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Conditions is a wrapper object for actual Condition functions to allow easier mocking/testing.
//go:generate mockgen -destination=../util/mocks/$GOPACKAGE/conditions.go -package=$GOPACKAGE -source conditions.go

type Conditions interface {
	SetCondition(conditions *[]v1alpha1.GitOpsDeploymentCondition, conditionType v1alpha1.GitOpsDeploymentConditionType,
		status v1alpha1.GitOpsConditionStatus, reason v1alpha1.GitOpsDeploymentReasonType, message string)
	FindCondition(conditions *[]v1alpha1.GitOpsDeploymentCondition,
		conditionType v1alpha1.GitOpsDeploymentConditionType) (*v1alpha1.GitOpsDeploymentCondition, bool)
	HasCondition(conditions *[]v1alpha1.GitOpsDeploymentCondition,
		conditionType v1alpha1.GitOpsDeploymentConditionType) bool
}

type ConditionManager struct {
}

// Implement functions for the interface

// SetCondition updates GitOpsDeployment status conditions
func (c *ConditionManager) SetCondition(conditions *[]v1alpha1.GitOpsDeploymentCondition, conditionType v1alpha1.GitOpsDeploymentConditionType, status v1alpha1.GitOpsConditionStatus, reason v1alpha1.GitOpsDeploymentReasonType, message string) {
	now := metav1.Now()
	condition, _ := c.FindCondition(conditions, conditionType)
	if message != condition.Message ||
		status != condition.Status ||
		reason != condition.Reason ||
		conditionType != condition.Type {

		condition.LastTransitionTime = &now
	}
	if message != "" {
		condition.Message = message
	}
	condition.LastProbeTime = now
	condition.Reason = reason
	condition.Status = status
}

// FindCondition finds the suitable Condition object by looking into the conditions list and returns true if already exists
// but, if none exists, it appends one and returns false
func (c *ConditionManager) FindCondition(conditions *[]v1alpha1.GitOpsDeploymentCondition, conditionType v1alpha1.GitOpsDeploymentConditionType) (*v1alpha1.GitOpsDeploymentCondition, bool) {
	for i, condition := range *conditions {
		if condition.Type == conditionType {
			return &(*conditions)[i], true
		}
	}

	// No such condition exists, so append it
	*conditions = append(*conditions, v1alpha1.GitOpsDeploymentCondition{Type: conditionType})

	return &(*conditions)[len(*conditions)-1], false
}

// HasCondition checks for the existence of a given Condition type
func (c *ConditionManager) HasCondition(conditions *[]v1alpha1.GitOpsDeploymentCondition, conditionType v1alpha1.GitOpsDeploymentConditionType) bool {
	for _, condition := range *conditions {
		if condition.Type == conditionType {
			return true
		}
	}
	return false
}

// NewConditionManager returns a ConditionManager object
func NewConditionManager() Conditions {
	return &ConditionManager{}
}
