# Task: Create ONNX Validation Script

**Epic:** #1 MVP
**Status:** To Do

## Description
Create a Python script to download, validate, and test the ONNX version of the CLIP model. This ensures the model is compatible with the Go ONNX runtime and provides a baseline for expected outputs.

## Location
`internal/scripts/`

## Acceptance Criteria
- Create a Python script `validate_onnx.py`.
- The script should use the `transformers` and `onnx` Python libraries.
- It should download a pre-trained CLIP model (e.g., `openai/clip-vit-base-patch32`).
- It should export the model to the ONNX format.
- It should run a sample image and a sample text through the ONNX model and print the resulting embeddings to confirm it works as expected.
- Add a `requirements.txt` file in the same directory for the Python dependencies.