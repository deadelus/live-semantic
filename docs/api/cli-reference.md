# CLI Reference

Complete reference for all LiveSemantic commands and options.

## Global Options

```bash
livesemantic [GLOBAL_OPTIONS] COMMAND [COMMAND_OPTIONS]
```

### Global Flags
- `--config, -c`: Configuration file path (default: `config/local.yaml`)
- `--verbose, -v`: Enable verbose logging
- `--quiet, -q`: Suppress non-essential output
- `--log-level`: Set log level (debug|info|warn|error)
- `--version`: Show version information
- `--help, -h`: Show help information

## Commands

### `realtime` - Real-time Video Analysis

Process live video streams with real-time semantic filtering.

```bash
livesemantic realtime [OPTIONS]
```

#### Options

**Source Configuration:**
- `--source, -s`: Video source (`cam0`, `cam1`, `rtmp://...`, `http://...`)
- `--resolution`: Video resolution (`480p`, `720p`, `1080p`, `auto`)
- `--fps`: Frames per second (1-60, default: 10)
- `--buffer-size`: Frame buffer size (default: 10)

**AI Configuration:**
- `--filter, -f`: Semantic filters (comma-separated)
- `--threshold, -t`: Confidence threshold (0.0-1.0, default: 0.7)
- `--model`: AI model to use (`ViT-B/32`, `ViT-L/14`, custom path)
- `--batch-size`: Inference batch size (default: 1)

**Output Options:**
- `--output, -o`: Output format (`console`, `json`, `webhook`)
- `--alert`: Alert channels (`console`, `webhook:URL`, `slack:TOKEN`)
- `--duration`: Maximum processing duration (`30s`, `5m`, `1h`)
- `--save-frames`: Save matching frames to directory

**Performance:**
- `--workers`: Number of worker threads (default: CPU cores)
- `--gpu`: Enable GPU acceleration (requires CUDA)
- `--cache`: Enable model caching

#### Examples

```bash
# Basic webcam monitoring
livesemantic realtime --source=cam0 --filter="person,vehicle"

# Security surveillance with alerts
livesemantic realtime \
  --source="rtmp://camera.local/stream" \
  --filter="unauthorized person,vehicle in restricted area" \
  --alert="webhook:https://api.company.com/alerts" \
  --threshold=0.8

# High-performance monitoring
livesemantic realtime \
  --source=cam0 \
  --filter="crowd gathering,violence,emergency" \
  --resolution=1080p \
  --fps=30 \
  --workers=8 \
  --gpu
```

### `batch` - Batch Video Processing

Process video files or directories with semantic analysis.

```bash
livesemantic batch [OPTIONS]
```

#### Options

**Input Configuration:**
- `--file, -f`: Single video file to process
- `--directory, -d`: Directory of video files
- `--pattern`: File pattern to match (default: `*.{mp4,avi,mov,mkv}`)
- `--recursive, -r`: Process directories recursively

**Processing Options:**
- `--filter`: Semantic filters (comma-separated)
- `--threshold`: Confidence threshold (0.0-1.0)
- `--model`: AI model to use
- `--start-time`: Start processing from time (e.g., `1m30s`)
- `--end-time`: Stop processing at time
- `--sample-rate`: Process every Nth frame (default: 1)

**Output Options:**
- `--output, -o`: Output format (`json`, `csv`, `xml`)
- `--report`: Save analysis report to file
- `--export-clips`: Extract matching video segments
- `--clip-duration`: Duration of exported clips (default: `5s`)
- `--output-dir`: Directory for output files

**Performance:**
- `--workers`: Number of parallel workers
- `--chunk-size`: Video processing chunk size
- `--memory-limit`: Memory usage limit (e.g., `2GB`)

#### Examples

```bash
# Analyze single video
livesemantic batch \
  --file="wedding.mp4" \
  --filter="applause,dancing,emotional moment" \
  --export-clips \
  --output=json

# Batch process directory
livesemantic batch \
  --directory="/video/library/" \
  --filter="sports,celebration,crowd" \
  --workers=4 \
  --recursive \
  --output=csv \
  --report=analysis_report.csv

# Extract specific segments
livesemantic batch \
  --file="security_footage.mp4" \
  --filter="unauthorized access,suspicious activity" \
  --start-time=2h30m \
  --end-time=6h00m \
  --export-clips \
  --clip-duration=10s
```

### `list-sources` - List Video Sources

List available video input sources.

```bash
livesemantic list-sources [OPTIONS]
```

#### Options
- `--format`: Output format (`table`, `json`, `plain`)
- `--test`: Test each source availability

#### Examples

```bash
# List all sources
livesemantic list-sources

# Test source availability
livesemantic list-sources --test --format=json
```

### `benchmark` - Performance Testing

Run performance benchmarks on your system.

```bash
livesemantic benchmark [OPTIONS]
```

#### Options
- `--duration`: Benchmark duration (default: `30s`)
- `--model`: Model to benchmark
- `--resolution`: Video resolution to test
- `--report`: Save benchmark results to file
- `--comparison`: Compare multiple models

#### Examples

```bash
# Quick benchmark
livesemantic benchmark --duration=10s

# Comprehensive benchmark
livesemantic benchmark \
  --duration=60s \
  --model=all \
  --resolution=1080p \
  --report=benchmark.json
```

### `health-check` - System Health

Check system health and dependencies.

```bash
livesemantic health-check [OPTIONS]
```

#### Options
- `--detailed`: Show detailed component status
- `--fix`: Attempt to fix common issues
- `--models`: Check AI model availability

### `export-config` - Export Configuration

Export current configuration to file.

```bash
livesemantic export-config [OPTIONS]
```

#### Options
- `--output`: Output file path
- `--format`: Configuration format (`yaml`, `json`, `toml`)
- `--template`: Export as template with comments

## Exit Codes

- `0`: Success
- `1`: General error
- `2`: Configuration error
- `3`: Model loading error
- `4`: Video source error
- `5`: Processing error
- `6`: Output error

## Environment Variables

- `LIVESEMANTIC_CONFIG`: Default configuration file path
- `LIVESEMANTIC_MODEL_PATH`: Default model directory
- `LIVESEMANTIC_LOG_LEVEL`: Default log level
- `LIVESEMANTIC_GPU`: Enable GPU by default (`true`/`false`)
- `LIVESEMANTIC_WORKERS`: Default number of workers

## Configuration File Format

See [Configuration Reference](config-reference.md) for detailed configuration options.