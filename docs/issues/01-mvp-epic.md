# Epic: MVP - Core Architecture & Real-time Pipeline

This epic tracks the completion of the Minimum Viable Product (MVP) for LiveSemantic. The goal is to build a working, end-to-end real-time video analysis pipeline that respects the project's Clean Architecture principles.

## Key Deliverables
- A `realtime` CLI command that can analyze a live webcam feed.
- Semantic matching of video frames against user-provided text filters.
- Console-based alerts for successful matches, including timestamp and filter details.
- A well-defined project structure with clear separation of concerns (Domain, Application, Infrastructure).

## Child Issues
- #2 Core Domain Models
- #3 Infrastructure Interfaces
- #4 ONNX Validation Script
- #5 Core Implementations (VideoSource, AIProvider, Alerter)
- #6 Real-time Analysis Use Case
- #7 `realtime` CLI Command