import torch
from transformers import CLIPProcessor, CLIPModel
from PIL import Image
import requests
import onnx
import onnxruntime as ort
import numpy as np
import os

# --- Configuration ---
MODEL_ID = "openai/clip-vit-base-patch32"
ONNX_PATH = "clip.onnx"
IMAGE_URL = "http://images.cocodataset.org/val2017/000000039769.jpg"
SAMPLE_TEXT = "a photo of a cat"

# --- Main Script ---
def export_to_onnx():
    """Downloads the CLIP model and exports it to ONNX format."""
    print(f"Loading model '{MODEL_ID}'...")
    model = CLIPModel.from_pretrained(MODEL_ID)
    processor = CLIPProcessor.from_pretrained(MODEL_ID)

    print("Preparing dummy inputs...")
    # Dummy inputs for text and vision encoders
    dummy_text_input = processor(text=[SAMPLE_TEXT], return_tensors="pt", padding=True)
    
    # Download a sample image
    image = Image.open(requests.get(IMAGE_URL, stream=True).raw)
    dummy_vision_input = processor(images=image, return_tensors="pt")

    # The model's forward pass takes both inputs
    inputs = {
        'input_ids': dummy_text_input['input_ids'],
        'attention_mask': dummy_text_input['attention_mask'],
        'pixel_values': dummy_vision_input['pixel_values'],
    }

    print(f"Exporting model to '{ONNX_PATH}'...")
    torch.onnx.export(
        model,
        (inputs['input_ids'], inputs['pixel_values'], inputs['attention_mask']),
        ONNX_PATH,
        input_names=['input_ids', 'pixel_values', 'attention_mask'],
        output_names=['logits_per_image', 'logits_per_text', 'text_embeds', 'image_embeds'],
        dynamic_axes={
            'input_ids': {0: 'batch_size', 1: 'sequence'},
            'attention_mask': {0: 'batch_size', 1: 'sequence'},
            'pixel_values': {0: 'batch_size'},
        },
        opset_version=14
    )
    print("Export complete.")

def validate_onnx_model():
    """Validates the exported ONNX model by running an inference."""
    if not os.path.exists(ONNX_PATH):
        print(f"'{ONNX_PATH}' not found. Please export the model first.")
        return

    print("\n--- Validating ONNX Model ---")
    print("Loading ONNX model...")
    ort_session = ort.InferenceSession(ONNX_PATH)

    print("Preparing inputs for ONNX runtime...")
    processor = CLIPProcessor.from_pretrained(MODEL_ID)
    
    # Process text
    text_inputs = processor(text=[SAMPLE_TEXT], return_tensors="np", padding=True)
    
    # Process image
    image = Image.open(requests.get(IMAGE_URL, stream=True).raw)
    image_inputs = processor(images=image, return_tensors="np")

    onnx_inputs = {
        "input_ids": text_inputs['input_ids'],
        "pixel_values": image_inputs['pixel_values'],
        "attention_mask": text_inputs['attention_mask'],
    }

    print("Running inference...")
    outputs = ort_session.run(None, onnx_inputs)
    
    image_embeds = outputs[3]
    text_embeds = outputs[2]

    print(f"Image embedding shape: {image_embeds.shape}")
    print(f"Text embedding shape: {text_embeds.shape}")
    
    # Normalize embeddings and calculate similarity
    image_embeds /= np.linalg.norm(image_embeds)
    text_embeds /= np.linalg.norm(text_embeds)
    dot_similarity = np.dot(image_embeds, text_embeds.T)

    print(f"\nSimilarity between image and '{SAMPLE_TEXT}': {dot_similarity[0][0]:.4f}")
    print("Validation successful: Model produced embeddings.")

if __name__ == "__main__":
    if not os.path.exists(ONNX_PATH):
        export_to_onnx()
    validate_onnx_model()