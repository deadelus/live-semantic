# Task: Implement Core Infrastructure Adapters

**Epic:** #1 MVP
**Status:** To Do

## Description
Create the concrete implementations (adapters) for the core infrastructure interfaces defined in #3. These will handle the actual interaction with external libraries and services.

## Location
`src/implementation/`

## Acceptance Criteria
- Create `webcam_source.go` which implements the `infrastructure.VideoSource` interface using the `gocv` library.
- Create `onnx_provider.go` which implements the `infrastructure.AIProvider` interface using a Go ONNX runtime library. It will load the model validated in #4.
- Create `console_alerter.go` which implements the `infrastructure.Alerter` interface. It should format the `MatchEvent` and print it to `stdout`.
- All implementations should be in their own files within the `implementation` package.
- These implementations will be injected into the use case in a later step.