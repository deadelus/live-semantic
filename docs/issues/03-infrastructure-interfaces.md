# Task: Define Infrastructure Interfaces

**Epic:** #1 MVP
**Status:** To Do

## Description
Define the interfaces (ports) for all external services and tools. These interfaces will be implemented by concrete adapters in the `implementation` layer and used by the `application` layer.

## Location
`src/infrastructure/`

## Acceptance Criteria
- Create `video_source.go` with a `VideoSource` interface. It should define methods like `NextFrame() (*models.Frame, error)` and `Close()`.
- Create `ai_provider.go` with an `AIProvider` interface. It should define methods like `EncodeText(string) (Embedding, error)` and `EncodeImage([]byte) (Embedding, error)`.
- Create `alerter.go` with an `Alerter` interface. It should define a method like `Alert(models.MatchEvent) error`.
- All interfaces should be in the `infrastructure` package.