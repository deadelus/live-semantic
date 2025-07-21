package dto_test

import (
	"live-semantic/src/domain/dto"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTaskRequest(t *testing.T) {
	t.Run("should create a task request", func(t *testing.T) {
		// Given
		title := "Test Task"
		description := "This is a test task"

		// When
		taskRequest := dto.TaskRequest{
			Title:       title,
			Description: description,
		}

		// Then
		assert.Equal(t, title, taskRequest.Title)
		assert.Equal(t, description, taskRequest.Description)
	})
}

func TestTaskResponse(t *testing.T) {
	t.Run("should create a task response", func(t *testing.T) {
		// Given
		id := "123"
		title := "Test Task"
		description := "This is a test task"
		createdAt := time.Now()

		// When
		taskResponse := dto.TaskResponse{
			ID:          id,
			Title:       title,
			Description: description,
			CreatedAt:   createdAt,
		}

		// Then
		assert.Equal(t, id, taskResponse.ID)
		assert.Equal(t, title, taskResponse.Title)
		assert.Equal(t, description, taskResponse.Description)
		assert.Equal(t, createdAt, taskResponse.CreatedAt)
	})
}
