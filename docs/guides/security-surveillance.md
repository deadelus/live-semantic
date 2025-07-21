# Security Surveillance Guide

Complete guide for setting up LiveSemantic for security and surveillance applications.

## Overview

LiveSemantic provides intelligent video surveillance capabilities that go beyond simple motion detection. By using semantic understanding, it can identify specific security events, reduce false alarms, and provide contextual alerts.

## Quick Setup

### Basic Security Monitoring

```bash
# Monitor entrance with person detection
livesemantic realtime \
  --source="rtmp://camera.local/entrance" \
  --filter="unauthorized person,person at night" \
  --threshold=0.8 \
  --alert="webhook:https://security.company.com/alerts"
```

## Advanced Security Filters

### Person Detection & Behavior
```bash
# Comprehensive person monitoring
--filter="person walking,person running,person hiding,person climbing,suspicious behavior"
```

### Vehicle Monitoring
```bash
# Vehicle and parking violations
--filter="vehicle in restricted area,unauthorized vehicle,vehicle at night,truck loading"
```

### Activity Detection
```bash
# Security events
--filter="crowd gathering,fight,violence,vandalism,break-in,theft"
```

### Time-based Filters
```bash
# After-hours monitoring
--filter="person after hours,vehicle after hours,activity after hours"
```

## Configuration Examples

### Entrance Monitoring

```yaml
# config/entrance_security.yaml
mode: "realtime"

video:
  source: "rtmp://entrance-camera/stream"
  resolution: "1080p"
  fps: 15
  buffer_size: 5

ai:
  model_path: "models/clip_ViT-L-14.onnx"  # High accuracy
  confidence_threshold: 0.75
  filters:
    - "unauthorized person entering"
    - "person with weapon"
    - "person forcing entry"
    - "multiple people entering together"

alerts:
  channels:
    - type: "webhook"
      url: "https://security.company.com/entrance-alerts"
      headers:
        Authorization: "Bearer YOUR_TOKEN"
    - type: "email"
      to: ["security@company.com"]
      subject: "Entrance Security Alert"

monitoring:
  working_hours: "09:00-17:00"
  timezone: "America/New_York"
  escalation_threshold: 3  # Alert after 3 consecutive matches
```

### Perimeter Security

```yaml
# config/perimeter_security.yaml
mode: "realtime"

video:
  source: "rtmp://perimeter-camera/stream"
  resolution: "720p"
  fps: 10

ai:
  confidence_threshold: 0.7
  filters:
    - "person climbing fence"
    - "person cutting fence"
    - "unauthorized vehicle"
    - "person in restricted area"
    - "suspicious activity"

alerts:
  channels:
    - type: "webhook"
      url: "https://security.company.com/perimeter-alerts"
      priority: "high"
    - type: "slack"
      webhook_url: "https://hooks.slack.com/services/YOUR/SLACK/WEBHOOK"
      channel: "#security-alerts"

output:
  save_clips: true
  clip_duration: "15s"
  clip_directory: "/security/footage/alerts/"
  retention_days: 30
```

### Parking Lot Surveillance

```bash
livesemantic realtime \
  --config="config/parking_security.yaml" \
  --source="rtmp://parking-camera/stream" \
  --filter="vehicle theft,vandalism,suspicious person,unauthorized parking" \
  --save-frames \
  --alert="webhook:https://security.company.com/parking-alerts"
```

## Multi-Camera Setup

### Camera Coordination

```bash
# Terminal 1 - Entrance
livesemantic realtime \
  --source="rtmp://entrance/stream" \
  --filter="person entering,unauthorized access" \
  --alert="webhook:https://api.company.com/alerts?camera=entrance" \
  --instance-id="entrance"

# Terminal 2 - Parking
livesemantic realtime \
  --source="rtmp://parking/stream" \
  --filter="vehicle theft,vandalism" \
  --alert="webhook:https://api.company.com/alerts?camera=parking" \
  --instance-id="parking"

# Terminal 3 - Perimeter
livesemantic realtime \
  --source="rtmp://perimeter/stream" \
  --filter="fence climbing,unauthorized access" \
  --alert="webhook:https://api.company.com/alerts?camera=perimeter" \
  --instance-id="perimeter"
```

### Centralized Management

```bash
# Use Docker Compose for multiple cameras
docker-compose -f deployments/security/docker-compose.yml up -d
```

## Alert Integration

### Webhook Payload Format

```json
{
  "timestamp": "2025-07-19T12:34:56Z",
  "camera_id": "entrance-001",
  "location": "Main Entrance",
  "alert_type": "security",
  "event": {
    "filter": "unauthorized person entering",
    "confidence": 0.89,
    "frame_number": 1245,
    "bbox": [120, 80, 200, 300]
  },
  "media": {
    "image_url": "https://storage.company.com/alerts/image_1245.jpg",
    "video_url": "https://storage.company.com/alerts/clip_1245.mp4"
  },
  "metadata": {
    "working_hours": false,
    "consecutive_alerts": 1,
    "severity": "high"
  }
}
```

### Integration Examples

**Slack Integration:**
```bash
# Add Slack webhook
livesemantic realtime \
  --source="cam0" \
  --filter="security breach" \
  --alert="slack:https://hooks.slack.com/services/YOUR/SLACK/WEBHOOK"
```

**Email Alerts:**
```yaml
alerts:
  email:
    smtp_server: "smtp.company.com"
    smtp_port: 587
    username: "alerts@company.com"
    password: "${EMAIL_PASSWORD}"
    to: ["security@company.com", "manager@company.com"]
    subject_template: "Security Alert: {filter} detected"
```

**Security System Integration:**
```bash
# Send to security management system
livesemantic realtime \
  --source="rtmp://camera/stream" \
  --filter="intrusion detected" \
  --alert="webhook:https://securitysystem.company.com/api/alerts" \
  --headers="Authorization: Bearer ${SECURITY_TOKEN}"
```

## Performance Optimization for Security

### High-Performance Setup

```yaml
# Optimized for 24/7 surveillance
video:
  fps: 10  # Balance between detection and performance
  resolution: "720p"  # Good balance for security
  buffer_size: 3  # Minimal latency

ai:
  model_path: "models/clip_ViT-B-32.onnx"  # Faster inference
  batch_size: 1  # Real-time processing
  confidence_threshold: 0.7

performance:
  workers: 4
  gpu_acceleration: true
  memory_limit: "2GB"
  
monitoring:
  enable_metrics: true
  metrics_port: 9090
```

### Resource Monitoring

```bash
# Monitor system resources
livesemantic realtime \
  --source="cam0" \
  --filter="security event" \
  --metrics-prometheus \
  --metrics-port=9090 \
  --health-check-interval=30s
```

## Security Best Practices

### Network Security
- Use HTTPS/TLS for all webhook communications
- Implement authentication tokens for API access
- Use VPN for remote camera access
- Regular security updates for camera firmware

### Data Privacy
- Configure data retention policies
- Encrypt stored video data
- Implement access controls
- Regular audit of access logs

### System Reliability
- Implement redundant storage
- Set up automated backups
- Monitor system health continuously
- Have failover procedures

### Alert Management
- Implement alert severity levels
- Set up escalation procedures
- Regular testing of alert systems
- Alert fatigue prevention

## Compliance Considerations

### GDPR Compliance
```yaml
privacy:
  data_retention_days: 30  # Adjust based on requirements
  anonymize_faces: true
  blur_license_plates: false
  audit_logging: true
```

### Industry Standards
- Follow CCTV code of practice
- Implement proper signage
- Regular privacy impact assessments
- Staff training on surveillance ethics

## Troubleshooting Security Setup

### Common Issues

**False Alarms:**
```bash
# Increase confidence threshold
livesemantic realtime --threshold=0.85

# Use more specific filters
--filter="unauthorized person entering building" # More specific
# vs
--filter="person" # Too general
```

**Missing Alerts:**
```bash
# Lower threshold for important areas
livesemantic realtime --threshold=0.6

# Add multiple related filters
--filter="break-in,forced entry,unauthorized access,suspicious activity"
```

**Performance Issues:**
```bash
# Optimize for security environment
livesemantic realtime \
  --resolution=720p \
  --fps=10 \
  --workers=2 \
  --model=ViT-B/32
```

## Advanced Features

### Zone-Based Monitoring
```yaml
zones:
  entrance:
    filters: ["unauthorized person", "weapon detection"]
    threshold: 0.8
  parking:
    filters: ["vehicle theft", "vandalism"]
    threshold: 0.7
  restricted:
    filters: ["any person", "any vehicle"]
    threshold: 0.6
```

### Behavioral Analysis
```bash
# Advanced behavioral patterns
livesemantic realtime \
  --filter="loitering,suspicious behavior,aggressive behavior,unusual activity"
```

### Integration with Existing Systems
- CCTV management systems
- Access control systems
- Alarm systems
- Security guard mobile apps

## Deployment Examples

See [Deployment Guide](../deployment/docker.md) for containerized security deployments.

## Next Steps

1. [Performance Tuning Guide](../troubleshooting/performance-tuning.md)
2. [Deployment Guide](../deployment/kubernetes.md)
3. [Monitoring Setup](../deployment/monitoring.md)