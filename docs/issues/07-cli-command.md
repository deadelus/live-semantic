# Task: Build `realtime` CLI Command

**Epic:** #1 MVP
**Status:** To Do

## Description
Create the user-facing `realtime` command using Cobra. This command will be responsible for initializing all the concrete implementations, injecting them into the use case, and executing it.

## Location
`src/transport/cli/` and `src/transport/cmd/`

## Acceptance Criteria
- Create a `cmd_realtime.go` file in `src/transport/cmd/`.
- Define a new Cobra command `realtimeCmd`.
- The command should accept arguments like `--source` and `--filter`.
- In the `Run` function of the command:
  - Instantiate the concrete `implementation.WebcamSource`, `implementation.ONNXProvider`, and `implementation.ConsoleAlerter`.
  - Instantiate the `uc.RealtimeAnalysisUseCase` with these dependencies.
  - Call the `Execute` method on the use case.
- Add the new command to the `root.go` file.