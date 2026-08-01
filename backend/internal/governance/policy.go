package governance

import (
	"errors"
	"strings"
)

type VoteValue string

const (
	VoteApprove VoteValue = "approve"
	VoteReject  VoteValue = "reject"
)

var (
	ErrInvalidVote                = errors.New("invalid vote")
	ErrRejectionReasonRequired    = errors.New("rejection reason is required")
	ErrRejectedProposalDependency = errors.New("approved proposal operation depends on a rejected operation")
	ErrUnresolvedRejection        = errors.New("unresolved rejection blocks publication")
)

type BlockingThread struct {
	ID       string
	Resolved bool
}

func ValidateVote(value VoteValue, reason string) error {
	switch value {
	case VoteApprove:
		return nil
	case VoteReject:
		if strings.TrimSpace(reason) == "" {
			return ErrRejectionReasonRequired
		}
		return nil
	default:
		return ErrInvalidVote
	}
}

func CanPublish(threads []BlockingThread) error {
	for _, thread := range threads {
		if !thread.Resolved {
			return ErrUnresolvedRejection
		}
	}
	return nil
}
