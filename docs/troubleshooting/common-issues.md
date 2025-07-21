# Common Issues & Solutions

Comprehensive guide to resolving common LiveSemantic issues.

## Installation Issues

### Issue: "Command not found: livesemantic"

**Symptoms:**
```bash
$ livesemantic --version
bash: livesemantic: command not found
```

**Solutions:**
1. **Check installation path:**
   ```bash
   which livesemantic
   echo $PATH
   ```

2. **Add to PATH:**
   ```bash
   # Add to ~/.bashrc or ~/.zshrc
   export PATH="/usr/local/bin:$PATH"
   source ~/.bashrc
   ```

3. **Reinstall globally:**
   ```bash
   sudo cp livesemantic /usr/local/bin/
   sudo chmod +x /usr/local/bin/livesemantic
   ```

### Issue: "ONNX Runtime library not found"

**Symptoms:**
```
Error: failed to load ONNX model: onnxruntime library not found
```

**Solutions:**
1. **Install ONNX Runtime:**
   ```bash
   # Ubuntu/Debian
   sudo apt install libonnxruntime-dev
   
   # macOS
   brew install onnxruntime
   
   # Or download from: https://github.com/microsoft/onnxruntime/releases
   ```

2. **Set library path:**
   ```bash
   export LD_LIBRARY_PATH="/usr/local/lib:$LD_LIBRARY_PATH"
   ```

3. **Verify installation:**
   ```bash
   ldconfig -p | grep onnx
   ```

### Issue: "OpenCV not found or incompatible version"

**Symptoms:**
```
Error: OpenCV version 4.x required, found 3.x
```

**Solutions:**
1. **Remove old OpenCV:**
   ```bash
   sudo apt purge libopencv*
   sudo apt autoremove
   ```

2. **Install OpenCV 4.x:**
   ```bash
   sudo apt update
   sudo apt install libopencv-dev libopencv-contrib-dev
   ```

3. **Verify version:**
   ```bash
   pkg-config --modversion opencv4
   ```

## Model Issues

### Issue: "Model file not found"

**Symptoms:**
```
Error: failed to load model: open models/clip_text.onnx: no such file or directory
```

**Solutions:**
1. **Export models:**
   ```bash
   cd scripts/
   python3 export_clip_onnx.py --output=../models/
   ```

2. **Download pre-exported models:**
   ```bash
   wget https://github.com/your-org/livesemantic/releases/latest/download/models.tar.gz
   tar -xzf models.tar.gz
   ```

3. **Check model path in config:**
   ```yaml
   ai:
     model_path: "./models/clip_text.onnx"  # Correct path
   ```

### Issue: "Model inference too slow"

**Symptoms:**
- Inference time > 100ms per frame
- High CPU usage
- Memory exhaustion

**Solutions:**
1. **Use smaller model:**
   ```bash
   # Use ViT-B/32 instead of ViT-L/14
   livesemantic realtime --model=ViT-B/32
   ```

2. **Reduce resolution:**
   ```bash
   livesemantic realtime --resolution=480p --fps=10
   ```

3. **Enable GPU acceleration:**
   ```bash
   livesemantic realtime --gpu --batch-size=4
   ```

4. **Optimize inference settings:**
   ```yaml
   ai:
     batch_size: 1
     num_threads: 4
     optimization_level: "all"
   ```

## Video Source Issues

### Issue: "Failed to open video source"

**Symptoms:**
```
Error: failed to open video source cam0: device not found
```

**Solutions:**
1. **List available sources:**
   ```bash
   livesemantic list-sources
   ls /dev/video*
   ```

2. **Check permissions:**
   ```bash
   sudo usermod -a -G video $USER
   # Logout and login again
   ```

3. **Test with different source:**
   ```bash
   # Try different camera index
   livesemantic realtime --source=cam1
   
   # Or use device path directly
   livesemantic realtime --source=/dev/video0
   ```

### Issue: "RTMP stream connection failed"

**Symptoms:**
```
Error: failed to connect to RTMP stream: connection timeout
```

**Solutions:**
1. **Verify stream URL:**
   ```bash
   # Test with ffmpeg
   ffmpeg -i rtmp://camera.local/stream -t 5 test.mp4
   ```

2. **Check network connectivity:**
   ```bash
   ping camera.local
   telnet camera.local 1935
   ```

3. **Add authentication if required:**
   ```bash
   livesemantic realtime --source="rtmp://user:pass@camera.local/stream"
   ```

4. **Increase timeout:**
   ```yaml
   video:
     source_timeout: 30s
     reconnect_attempts: 5
   ```

## Performance Issues

### Issue: "High memory usage"

**Symptoms:**
- Memory usage continuously increasing
- Out of memory errors
- System slowdown

**Solutions:**
1. **Reduce buffer size:**
   ```yaml
   video:
     buffer_size: 5  # Reduce from default 10
   ```

2. **Limit batch size:**
   ```bash
   livesemantic realtime --batch-size=1
   ```

3. **Enable garbage collection tuning:**
   ```bash
   export GOGC=50  # More aggressive GC
   ```

4. **Monitor memory usage:**
   ```bash
   livesemantic realtime --metrics-prometheus --metrics-port=9090
   ```

### Issue: "CPU usage too high"

**Symptoms:**
- CPU at 100%
- Frame drops
- Processing lag

**Solutions:**
1. **Reduce worker threads:**
   ```bash
   livesemantic realtime --workers=2
   ```

2. **Lower FPS:**
   ```bash
   livesemantic realtime --fps=5
   ```

3. **Use hardware acceleration:**
   ```bash
   livesemantic realtime --gpu
   ```

4. **Profile performance:**
   ```bash
   livesemantic benchmark --duration=30s --detailed
   ```

## Alert & Output Issues

### Issue: "Webhook alerts not working"

**Symptoms:**
- No webhook calls received
- Alert failures in logs

**Solutions:**
1. **Test webhook manually:**
   ```bash
   curl -X POST https://api.company.com/alerts \
     -H "Content-Type: application/json" \
     -d '{"test": "alert"}'
   ```

2. **Check webhook configuration:**
   ```yaml
   alerts:
     webhook:
       url: "https://api.company.com/alerts"
       timeout: 10s
       retry_attempts: 3
   ```

3. **Enable webhook debugging:**
   ```bash
   livesemantic realtime --log-level=debug --filter="person"
   ```

### Issue: "JSON output malformed"

**Symptoms:**
- Invalid JSON in output files
- Parsing errors in downstream tools

**Solutions:**
1. **Validate JSON output:**
   ```bash
   jq . output.json  # Check for syntax errors
   ```

2. **Use streaming JSON for large outputs:**
   ```bash
   livesemantic batch --output=jsonlines
   ```

3. **Check disk space:**
   ```bash
   df -h  # Ensure sufficient disk space
   ```

## Debug Mode

### Enable Comprehensive Debugging

```bash
livesemantic realtime \
  --log-level=debug \
  --verbose \
  --source=cam0 \
  --filter="person" \
  --duration=30s
```

### Collect System Information

```bash
# Generate debug report
livesemantic health-check --detailed > debug_report.txt

# Include system information
echo "=== System Info ===" >> debug_report.txt
uname -a >> debug_report.txt
lscpu >> debug_report.txt
free -h >> debug_report.txt
```

## Getting Additional Help

1. **Check logs:** Enable debug logging for detailed error information
2. **GitHub Issues:** [Report bugs](https://github.com/your-org/livesemantic/issues)
3. **Discussions:** [Community support](https://github.com/your-org/livesemantic/discussions)
4. **Email Support:** support@livesemantic.io

## Performance Optimization Checklist

- [ ] Use appropriate model for your use case (ViT-B/32 for speed, ViT-L/14 for accuracy)
- [ ] Set optimal resolution and FPS for your hardware
- [ ] Configure appropriate worker thread count
- [ ] Enable GPU acceleration if available
- [ ] Monitor memory usage and adjust buffer sizes
- [ ] Use efficient output formats (JSON vs console)
- [ ] Optimize alert configurations
- [ ] Regular system maintenance and updates