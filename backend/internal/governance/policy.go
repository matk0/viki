package governance

import (
	"errors"
	"strings"
)

var (
	ErrObjectionReasonRequired  = errors.New("objection reason is required")
	ErrUnresolvedObjection      = errors.New("unresolved objection blocks approval")
	ErrParentFeatureNotApproved = errors.New("parent feature must be approved before its scenario")
)

type ObjectionState struct {
	ID       string
	Resolved bool
}

func ValidateObjectionReason(reason string) error {
	if strings.TrimSpace(reason) == "" {
		return ErrObjectionReasonRequired
	}
	return nil
}

func CanApprove(objections []ObjectionState) error {
	for _, objection := range objections {
		if !objection.Resolved {
			return ErrUnresolvedObjection
		}
	}
	return nil
}
