# First Steps Tutorial

This tutorial will guide you through your first LiveSemantic video analysis in 10 minutes.

## Quick Test with Webcam

### Step 1: Test Connection

```bash
# List available video sources
livesemantic list-sources

# Expected output:
# cam0: USB Camera (1920x1080)
# cam1: Integrated Webcam (640x480)
```

### Step 2: Basic Person Detection

```bash
# Start real-time person detection
livesemantic realtime \
  --source="cam0" \
  --filter="person" \
  --threshold=0.6 \
  --output=console

# You should see output like:
# [12:34:56] MATCH: person (confidence: 0.87) at frame 145
# [12:34:57] MATCH: person (confidence: 0.92) at frame 175
```

### Step 3: Multiple Filters

```bash
# Detect multiple objects
livesemantic realtime \
  --source="cam0" \
  --filter="person walking,person sitting,dog,cat" \
  --threshold=0.7 \
  --duration=30s
```

## Working with Video Files

### Step 1: Download Sample Video

```bash
# Create test directory
mkdir test-videos && cd test-videos

# Download sample (replace with actual video)
wget https://sample-videos.com/zip/10/mp4/SampleVideo_720x480_2mb.mp4 -O sample.mp4
```

### Step 2: Analyze Sample Video

```bash
# Basic analysis
livesemantic batch \
  --file="sample.mp4" \
  --filter="person,vehicle,building" \
  --threshold=0.6 \
  --output=json \
  --report=analysis.json

# View results
cat analysis.json | jq '.matches[] | select(.confidence > 0.8)'
```

### Step 3: Extract Clips

```bash
# Extract matching segments
livesemantic batch \
  --file="sample.mp4" \
  --filter="person walking,car driving" \
  --export-clips \
  --clip-duration=5s \
  --output-dir="extracted_clips/"
```

## Understanding Output

### Console Output Format
```
[HH:MM:SS] MATCH: <filter> (confidence: <score>) at frame <number>
[HH:MM:SS] INFO: Processed <frames> frames in <time>
[HH:MM:SS] STATS: <matches> matches, avg confidence: <score>
```

### JSON Output Format
```json
{
  "metadata": {
    "file": "sample.mp4",
    "duration": 120.5,
    "fps": 30,
    "total_frames": 3615
  },
  "matches": [
    {
      "timestamp": 15.2,
      "frame": 456,
      "filter": "person walking",
      "confidence": 0.89,
      "bbox": [120, 80, 200, 300]
    }
  ],
  "statistics": {
    "total_matches": 15,
    "avg_confidence": 0.76,
    "processing_time": 45.2
  }
}
```

## Common Workflows

### Security Monitoring
```bash
# Monitor entrance with alerts
livesemantic realtime \
  --source="rtmp://camera.local/entrance" \
  --filter="unauthorized person,vehicle at night" \
  --alert="webhook:https://api.company.com/alerts" \
  --working-hours="09:00-17:00"
```

### Content Creation
```bash
# Find highlights in event video
livesemantic batch \
  --file="wedding.mp4" \
  --filter="applause,dancing,emotional moment,group photo" \
  --export-clips \
  --min-duration=3s \
  --quality=high
```

### Video Search
```bash
# Index video library
livesemantic batch \
  --directory="/video/library/" \
  --filter="sports,music,celebration" \
  --index-database \
  --workers=4 \
  --recursive
```

## Performance Tips

### Optimize for Speed
- Use lower resolution: `--resolution=480p`
- Reduce FPS: `--fps=10`
- Lower threshold: `--threshold=0.5`
- Use ViT-B/32 model for faster inference

### Optimize for Accuracy
- Use higher resolution: `--resolution=1080p`
- Increase FPS: `--fps=30`
- Higher threshold: `--threshold=0.8`
- Use ViT-L/14 model for better accuracy

## What's Next?

1. [Advanced Configuration](configuration.md)
2. [Security Surveillance Guide](../guides/security-surveillance.md)
3. [Content Creation Guide](../guides/content-creation.md)
4. [API Reference](../api/cli-reference.md)

## Getting Help

- Check [Common Issues](../troubleshooting/common-issues.md)
- Join our [Discord Community](https://discord.gg/livesemantic)
- Read the [FAQ](../troubleshooting/faq.md)