package uc_test

import (
	"context"
	"live-semantic/src/domain/dto"
	"live-semantic/src/domain/uc"
	"testing"
	"time"

	"github.com/deadelus/go-clean-app/src/logger"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestUseCase_CreateTask(t *testing.T) {
	// Create a new mock controller
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create a new mock logger
	mockLogger := logger.NewMockLogger(ctrl)

	// Expect the Info method to be called
	mockLogger.EXPECT().Info(gomock.Any(), gomock.Any()).Return()

	// Create a new use case with the mock logger
	useCase, err := uc.NewUseCase(mockLogger)
	assert.NoError(t, err)

	// Create a new task request
	taskRequest := dto.TaskRequest{
		Title:       "Test Task",
		Description: "This is a test task",
	}

	// Call the CreateTask method
	result, err := useCase.CreateTask(context.Background(), taskRequest)
	assert.NoError(t, err)

	// Assert that the result is successful
	assert.True(t, result.Success)
	assert.NotNil(t, result.Data)
	assert.Equal(t, "12345", result.Data.ID)
	assert.Equal(t, "Task 1", result.Data.Title)
	assert.Equal(t, "Description 1", result.Data.Description)
	assert.WithinDuration(t, time.Now(), result.Data.CreatedAt, time.Second)
}

func TestUseCase_CreateTask_ContextCancelled(t *testing.T) {
	// Create a new mock controller
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create a new mock logger
	mockLogger := logger.NewMockLogger(ctrl)

	// Create a new use case with the mock logger
	useCase, err := uc.NewUseCase(mockLogger)
	assert.NoError(t, err)

	// Create a new task request
	taskRequest := dto.TaskRequest{
		Title:       "Test Task",
		Description: "This is a test task",
	}

	// Create a context and cancel it
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Call the CreateTask method
	result, err := useCase.CreateTask(ctx, taskRequest)
	assert.Error(t, err)
	assert.Equal(t, context.Canceled, err)

	// Assert that the result is a failure
	assert.False(t, result.Success)
	assert.Nil(t, result.Data)
	assert.Equal(t, "context cancelled", result.Error)
}
