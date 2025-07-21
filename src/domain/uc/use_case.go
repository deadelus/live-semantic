// Package domain contains the business logic and use cases for the application.
package uc

import (
	"context"
	"live-semantic/src/domain/dto"

	"github.com/deadelus/go-clean-app/src/logger"
)

// UseCases defines the interface for the use cases in the application.
type UseCases interface {
	CreateTask(context.Context, dto.TaskRequest) (dto.Result[dto.TaskResponse], error)
}

// useCase implements the UseCases interface.
type UseCase struct {
	logger logger.Logger
}

// NewUseCase initializes your use cases with all the necessary dependencies
func NewUseCase(logger logger.Logger) (UseCases, error) {
	return &UseCase{
		logger: logger,
	}, nil
}
