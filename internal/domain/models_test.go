package domain

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestOperationStatusAllowsOnlyLegalTransitions(t *testing.T) {
	require.True(t, StatusPendingApproval.CanTransitionTo(StatusApproved))
	require.True(t, StatusApproved.CanTransitionTo(StatusQueued))
	require.True(t, StatusQueued.CanTransitionTo(StatusRunning))
	require.True(t, StatusRunning.CanTransitionTo(StatusSucceeded))
	require.False(t, StatusPendingApproval.CanTransitionTo(StatusRunning))
	require.False(t, StatusSucceeded.CanTransitionTo(StatusRunning))
}

func TestValidateApproverRejectsRequester(t *testing.T) {
	requester := uuid.New()
	require.ErrorIs(t, ValidateApprover(requester, requester), ErrSelfApproval)
	require.NoError(t, ValidateApprover(requester, uuid.New()))
}
