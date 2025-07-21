# Installation Guide

## System Requirements

### Minimum Requirements
- **OS**: Linux (Ubuntu 20.04+), macOS (10.15+), Windows 10+
- **RAM**: 2GB minimum, 4GB recommended
- **Storage**: 1GB for models and binaries
- **CPU**: x64 architecture with AVX support

### Recommended Requirements
- **RAM**: 8GB+ for high-throughput processing
- **GPU**: NVIDIA GPU with CUDA 11.0+ (optional)
- **Storage**: SSD for model loading performance

## Prerequisites

### System Dependencies

**Ubuntu/Debian:**
```bash
sudo apt update
sudo apt install -y wget curl build-essential cmake
sudo apt install -y libopencv-dev python3 python3-pip
```

**macOS:**
```bash
brew install opencv cmake python3
```

**Windows:**
- Install Visual Studio Build Tools 2019+
- Install Python 3.9+ from python.org
- Install OpenCV via vcpkg or pre-built binaries

### Python Dependencies (for model export)
```bash
pip3 install torch torchvision transformers onnx onnxruntime
```

## Installation Methods

### Method 1: Binary Release (Recommended)

```bash
# Download latest release
wget https://github.com/your-org/livesemantic/releases/latest/download/livesemantic-linux-amd64
chmod +x livesemantic-linux-amd64
sudo mv livesemantic-linux-amd64 /usr/local/bin/livesemantic

# Verify installation
livesemantic --version
```

### Method 2: Build from Source

```bash
# Clone repository
git clone https://github.com/your-org/livesemantic.git
cd livesemantic

# Install Go dependencies
make deps

# Build binary
make build

# Install globally (optional)
sudo cp bin/livesemantic /usr/local/bin/
```

### Method 3: Docker

```bash
# Pull image
docker pull livesemantic/livesemantic:latest

# Run with webcam access
docker run -it --rm \
  --device=/dev/video0 \
  -v $(pwd)/config:/app/config \
  livesemantic/livesemantic:latest
```

## Model Setup

### Export ONNX Models

```bash
# Navigate to scripts directory
cd scripts/

# Export CLIP models (required)
python3 export_clip_onnx.py --model=ViT-B/32 --output=../models/
python3 export_clip_onnx.py --model=ViT-L/14 --output=../models/

# Verify models
ls -la ../models/
```

### Download Pre-exported Models

```bash
# Download from releases
wget https://github.com/your-org/livesemantic/releases/latest/download/models.tar.gz
tar -xzf models.tar.gz
```

## Verification

### Test Installation

```bash
# Check version and dependencies
livesemantic --version
livesemantic health-check

# Test with sample video
livesemantic batch \
  --file="test/sample.mp4" \
  --filter="person walking" \
  --dry-run
```

### Performance Test

```bash
# Benchmark on your system
livesemantic benchmark \
  --duration=30s \
  --model=ViT-B/32 \
  --report=performance.json
```

## Troubleshooting Installation

### Common Issues

**Error: "ONNX Runtime not found"**
```bash
# Linux/macOS
export LD_LIBRARY_PATH=/usr/local/lib:$LD_LIBRARY_PATH

# Or install system-wide
sudo apt install onnxruntime  # Ubuntu
brew install onnxruntime      # macOS
```

**Error: "OpenCV not found"**
```bash
# Verify OpenCV installation
pkg-config --modversion opencv4

# If missing, reinstall
sudo apt purge libopencv* 
sudo apt install libopencv-dev
```

**Error: "Permission denied" on video devices**
```bash
# Add user to video group
sudo usermod -a -G video $USER
# Logout and login again
```

## Next Steps

1. [First Steps Tutorial](first-steps.md)
2. [Configuration Guide](configuration.md)
3. [Security Setup Guide](../guides/security-surveillance.md)