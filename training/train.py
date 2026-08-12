#!/usr/bin/env python3
"""Fine-tunes YOLO11s from its COCO-pretrained checkpoint to add "hat" as
an 81st class, using the dataset prepare_dataset.py produces.

TODO.md § A, docs/adr/clip-backend.md § 24. See training/README.md's
"Reality check" — not run end-to-end in this repo's dev environment.

IMPORTANT CAVEAT, read before trusting the result (docs/adr/clip-backend.md
§ 24's own note): prepare_dataset.py only pulls "Hat"-labeled images —
none of the original 80 COCO classes' bounding boxes are present in that
data, because Open Images' own annotations for co-occurring objects in
those images weren't pulled. Training a fresh 81-class head using only
hat-labeled images risks catastrophic forgetting on the other 80 classes
(the model sees images where "person"/"car"/etc. may be visible but
never labeled, and can learn to stop finding them). This script mitigates
that (freezing most of the backbone + a low learning rate + few epochs,
all standard practice for a narrow single-class addition) but does NOT
eliminate the risk. The properly correct fix — mixing in a real COCO
subset with its original annotations so the model keeps seeing genuine
examples of the other 80 classes during fine-tuning — needs real COCO
detection data (large, not pulled here given this environment's disk
constraints, see README.md). Evaluate on a COCO validation subset before
trusting this for anything beyond "hat" detection specifically.
"""

from pathlib import Path

import yaml
from ultralytics import YOLO

# Must match internal/domain/entities/class.go's Yolo11sClasses() order —
# the ONNX model's output class indices are positional, not name-keyed,
# so the Go side and this list have to agree exactly.
COCO_CLASSES = [
    "person", "bicycle", "car", "motorcycle", "airplane", "bus", "train",
    "truck", "boat", "traffic light", "fire hydrant", "stop sign",
    "parking meter", "bench", "bird", "cat", "dog", "horse", "sheep",
    "cow", "elephant", "bear", "zebra", "giraffe", "backpack", "umbrella",
    "handbag", "tie", "suitcase", "frisbee", "skis", "snowboard",
    "sports ball", "kite", "baseball bat", "baseball glove", "skateboard",
    "surfboard", "tennis racket", "bottle", "wine glass", "cup", "fork",
    "knife", "spoon", "bowl", "banana", "apple", "sandwich", "orange",
    "broccoli", "carrot", "hot dog", "pizza", "donut", "cake", "chair",
    "couch", "potted plant", "bed", "dining table", "toilet", "tv",
    "laptop", "mouse", "remote", "keyboard", "cell phone", "microwave",
    "oven", "toaster", "sink", "refrigerator", "book", "clock", "vase",
    "scissors", "teddy bear", "hair drier", "toothbrush",
]
assert len(COCO_CLASSES) == 80, f"expected 80 COCO classes, got {len(COCO_CLASSES)}"

DATASET_DIR = Path(__file__).parent / "dataset"
BASE_CHECKPOINT = "yolo11s.pt"  # Ultralytics auto-downloads the COCO-pretrained weights

# Mitigation for the catastrophic-forgetting risk described above — not a
# substitute for real COCO data mixed into training, see the module
# docstring. freeze=10 keeps the first 10 layers (most of the backbone)
# fixed; low epochs/lr limit how far the rest can drift from the
# pretrained weights while still learning the new class.
FREEZE_LAYERS = 10
EPOCHS = 20
LEARNING_RATE = 0.001


def build_merged_data_yaml() -> Path:
    """Rewrites dataset/data.yaml (single class: hat) into an 81-class
    config — COCO_CLASSES + hat — pointing at the same hat-only images.
    Ultralytics only needs label files that reference valid class
    indices; images with no COCO-class labels simply contribute no
    gradient for those classes on this pass, which is the whole
    forgetting risk flagged above, not fixed by this rewrite alone.
    """
    src = DATASET_DIR / "data.yaml"
    if not src.exists():
        raise FileNotFoundError(f"{src} not found — run prepare_dataset.py first")

    merged = yaml.safe_load(src.read_text())
    names = {i: name for i, name in enumerate(COCO_CLASSES)}
    names[len(COCO_CLASSES)] = "hat"
    merged["names"] = names
    merged["nc"] = len(names)

    # prepare_dataset.py's export wrote "hat" as class index 0 in its own
    # label files (single-class dataset) — remap to index 80 (after the
    # 80 COCO classes) to match the merged config, otherwise "hat"
    # bounding boxes would be silently mislabeled as "person" (COCO
    # index 0) during training.
    hat_index = len(COCO_CLASSES)
    for split_dir in ("train", "val"):
        labels_dir = DATASET_DIR / split_dir / "labels"
        if not labels_dir.exists():
            continue
        for label_file in labels_dir.glob("*.txt"):
            lines = label_file.read_text().splitlines()
            remapped = []
            for line in lines:
                parts = line.split()
                if not parts:
                    continue
                parts[0] = str(hat_index)
                remapped.append(" ".join(parts))
            label_file.write_text("\n".join(remapped) + ("\n" if remapped else ""))

    merged_path = DATASET_DIR / "data_merged.yaml"
    merged_path.write_text(yaml.safe_dump(merged, sort_keys=False))
    print(f"Wrote {merged_path} (nc={merged['nc']}, hat at index {hat_index})")
    return merged_path


def main() -> None:
    data_yaml = build_merged_data_yaml()

    model = YOLO(BASE_CHECKPOINT)
    model.train(
        data=str(data_yaml),
        epochs=EPOCHS,
        freeze=FREEZE_LAYERS,
        lr0=LEARNING_RATE,
        imgsz=640,  # must match modelHeight/modelWidth in yolo11s.go
    )
    print("Training done — weights under runs/detect/train*/weights/best.pt")
    print("Next: export to ONNX (see README.md step 3), evaluate before trusting it.")


if __name__ == "__main__":
    main()
