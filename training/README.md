# Fine-tuning YOLO11s — pipeline

TODO.md § A ("pas d'OWL-ViT/GroundingDINO — piste retenue pour 'hat' et
attachements similaires = fine-tuning YOLO"), `docs/adr/clip-backend.md`
§ 24. This directory is the offline training pipeline decided there —
this Go repo is an **inference** runtime (ONNX Runtime + gocv), it has no
training capability itself, so extending YOLO11s's vocabulary happens
here, outside the Go build, producing a new `.onnx` that drops into
`internal/implementation/inference/onnx/yolo11s/` unchanged.

## Reality check — read before running anything

**Not executed in the environment this was written in** (checked
2026-08-12): Intel i7 (6-core) + AMD Radeon Pro 560X, no CUDA-capable
GPU, 16 GB RAM, ~8 GB free disk. Fine-tuning YOLO — even a small
single-class addition — realistically needs a CUDA GPU (Apple Silicon
Macs can use MPS via PyTorch, but this machine's discrete AMD GPU can't)
and tens of GB of free disk for the dataset + PyTorch/ultralytics
install. Every script here is written to be correct against the real
Ultralytics/FiftyOne APIs, but **none of it has actually been run
end-to-end** — treat it as a starting point to run on a machine that has
the hardware, not as verified working code. Test each step in isolation
before trusting the final `.onnx`.

## Prerequisites

- Python 3.10+ (this repo's dev machine had 3.14, untested with these
  package versions — pin what actually resolves on your machine)
- A CUDA GPU strongly recommended (or Apple Silicon + MPS). CPU-only
  training is possible but slow (hours to days depending on dataset size
  and epoch count).
- `pip install -r requirements.txt`

## Steps

1. **`prepare_dataset.py`** — pulls a subset of Open Images V7's "Hat"
   class (already has bounding-box annotations, no manual labeling
   needed) via FiftyOne, exports to YOLO format (`images/`, `labels/`,
   `data.yaml`). Adjust `MAX_SAMPLES`/`SPLITS` at the top of the script
   before running — the default is intentionally small (a few hundred
   images) to keep this tractable on a laptop, not tuned for accuracy.

   ```bash
   python prepare_dataset.py
   ```

2. **`train.py`** — fine-tunes from the same `yolo11s` checkpoint family
   already used by this project (Ultralytics' pretrained COCO weights,
   see its own comments for exactly which), extending the class list from
   80 to 81 (COCO + "hat"). See the script's own docstring for the
   catastrophic-forgetting caveat (fine-tuning on hat-only data, without
   mixing in original COCO images, risks degrading the other 80 classes —
   mitigated here with a frozen backbone + low learning rate + few
   epochs, not eliminated).

   ```bash
   python train.py
   ```

3. **Export to ONNX**, opset matching this project's runtime (opset 19,
   IR version 9 as of `yolo11s.go`'s own comment — re-check that comment
   hasn't drifted before assuming it's still current):

   ```bash
   python -c "from ultralytics import YOLO; YOLO('runs/detect/train/weights/best.pt').export(format='onnx', opset=19)"
   ```

4. **Update the Go side** — `internal/domain/entities/class.go`
   (`Yolo11sClasses()`, add `Class_Hat = "hat"`), `yolo11s.go`
   (`modelClasses = 81`), replace `assets/models/yolo11s.onnx`. Not
   automated here — deliberately a manual, reviewed step given how
   central this model is to the whole pipeline.

## What's NOT done

- The dataset script pulls "Hat" only — no other accessories (backpack,
  glasses, etc. are already COCO classes; other non-COCO accessories
  would need their own class added to `data.yaml` and re-training).
- No accuracy/regression evaluation against the original 80 COCO classes
  after fine-tuning — needed before trusting the result in production,
  not built here.
- No CI/automation — this is a manual, occasional pipeline, not run on
  every commit.
