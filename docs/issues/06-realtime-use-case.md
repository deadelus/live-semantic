# Task: Implement Real-time Analysis Use Case

**Epic:** #1 MVP
**Status:** To Do

## Description
Create the use case that contains the core application logic for the real-time analysis pipeline. This use case will orchestrate the interactions between the domain models and the infrastructure interfaces.

## Location
`src/domain/uc/`

## Acceptance Criteria
- Create `uc_realtime_analysis.go`.
- Define a `RealtimeAnalysisUseCase` struct that takes the `infrastructure.VideoSource`, `infrastructure.AIProvider`, and `infrastructure.Alerter` interfaces as dependencies via its constructor (dependency injection).
- Create an `Execute` method on the use case that:
  - Continuously reads frames from the `VideoSource`.
  - For each frame, gets the image embedding from the `AIProvider`.
  - Compares the image embedding to the text filter's embedding.
  - If they match, creates a `models.MatchEvent`.
  - Sends the `MatchEvent` to the `Alerter`.
- The use case must only depend on `domain` and `infrastructure` (interfaces), not on `implementation`.