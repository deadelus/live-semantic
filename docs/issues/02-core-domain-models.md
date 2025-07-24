# Task: Define Core Domain Models

**Epic:** #1 MVP
**Status:** To Do

## Description
Define the core business objects for the application. These models should be pure data structures with no dependencies on external layers.

## Location
`src/domain/models/`

## Acceptance Criteria
- Create `frame.go` to represent a single video frame (e.g., with image data, timestamp).
- Create `filter.go` to represent a user-defined semantic filter (e.g., "a person walking").
- Create `match_event.go` to represent a successful match between a frame and a filter (e.g., with frame reference, filter reference, confidence score).
- All models should be in the `models` package.
- All models must be pure and not import from `application` or `infrastructure`.