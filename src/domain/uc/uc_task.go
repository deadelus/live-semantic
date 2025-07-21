package uc

import (
	"context"
	"live-semantic/src/domain/dto"
	"time"
)

func (uc *UseCase) CreateTask(ctx context.Context, er dto.TaskRequest) (dto.Result[dto.TaskResponse], error) {
	// Check if the context is done before proceeding
	select {
	case <-ctx.Done():
		return dto.Failure[dto.TaskResponse]("context cancelled"), ctx.Err()
	default:
	}

	/*
		Here you would typically interact with your repositories or services
		to perform the business logic. For example, you might create a user
		in a database and return the created user as a response.
	*/

	// Log the request for debugging purposes
	uc.logger.Info("Processing Task use case", map[string]interface{}{
		"request": er,
	})

	// For demonstration, let's return a dummy response.
	response := dto.TaskResponse{
		ID:          "12345",
		Title:       "Task 1",
		Description: "Description 1",
		CreatedAt:   time.Now(),
	}
	return dto.Success(response), nil
}
