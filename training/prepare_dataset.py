#!/usr/bin/env python3
"""Pulls a subset of Open Images V7's "Hat" class via FiftyOne and exports
it to YOLO format (images/ + labels/ + data.yaml), ready for
ultralytics.YOLO.train().

TODO.md § A, docs/adr/clip-backend.md § 24 — see training/README.md's
"Reality check" before running this: written against the real FiftyOne
API, not executed end-to-end in this repo's dev environment (no spare
disk/no GPU to make use of the result anyway).

Open Images V7 already has "Hat" as an annotated detection class — no
manual labeling needed, unlike a from-scratch dataset.
"""

from pathlib import Path

import fiftyone as fo
import fiftyone.zoo as foz

# Kept small on purpose — this is a starting point to prove the pipeline
# end-to-end on modest hardware, not a dataset sized for real accuracy.
# Bump once the rest of the pipeline (train.py, ONNX export, Go-side
# wiring) is confirmed working.
CLASSES = ["Hat"]
SPLITS = ["train", "validation"]
MAX_SAMPLES = {"train": 400, "validation": 80}
OUTPUT_DIR = Path(__file__).parent / "dataset"


def main() -> None:
    OUTPUT_DIR.mkdir(exist_ok=True)

    for split in SPLITS:
        print(f"Downloading Open Images V7 ({split}) subset for classes={CLASSES}...")
        dataset = foz.load_zoo_dataset(
            "open-images-v7",
            split=split,
            label_types=["detections"],
            classes=CLASSES,
            max_samples=MAX_SAMPLES[split],
            dataset_name=f"livesemantic-hat-{split}",
        )

        # Open Images' own label set includes many classes beyond CLASSES
        # in the same images (co-occurring objects) — restrict exported
        # labels to just what we asked for, otherwise the "hat" dataset
        # would silently teach the model dozens of other unrelated
        # classes with incomplete/inconsistent annotation coverage.
        view = dataset.filter_labels("detections", fo.ViewField("label").is_in(CLASSES))

        export_dir = OUTPUT_DIR / ("val" if split == "validation" else split)
        print(f"Exporting {len(view)} samples to {export_dir} (YOLO format)...")
        view.export(
            export_dir=str(export_dir),
            dataset_type=fo.types.YOLOv5Dataset,
            label_field="detections",
            classes=CLASSES,
        )

    data_yaml = OUTPUT_DIR / "data.yaml"
    data_yaml.write_text(
        f"path: {OUTPUT_DIR.resolve()}\n"
        "train: train/images\n"
        "val: val/images\n"
        f"names:\n  0: hat\n"
    )
    print(f"Wrote {data_yaml}")
    print(
        "NOTE: this data.yaml declares a single class (hat, index 0) — "
        "train.py merges it against the original 80 COCO classes at "
        "training time (see its own docstring), not here."
    )


if __name__ == "__main__":
    main()
