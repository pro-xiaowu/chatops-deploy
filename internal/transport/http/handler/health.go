package handler

import "context"

type ReadinessCheck func(context.Context) error

type Health struct {
	readiness ReadinessCheck
}

type Status struct {
	Status string `json:"status"`
}

func NewHealth(readiness ReadinessCheck) *Health {
	if readiness == nil {
		readiness = func(context.Context) error { return nil }
	}
	return &Health{readiness: readiness}
}

func (h *Health) Liveness() Status {
	return Status{Status: "ok"}
}

func (h *Health) Readiness(ctx context.Context) (Status, error) {
	if err := h.readiness(ctx); err != nil {
		return Status{}, err
	}
	return Status{Status: "ok"}, nil
}
