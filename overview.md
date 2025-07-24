# 🎯 **LiveSemantic - Architecture & Projet**

## 📋 **Vue d'ensemble**

**Analyseur sémantique vidéo temps réel** avec filtres IA en langage naturel, optimisé pour performance maximale et déploiement agnostique.

---

## 🏗️ **Architecture Clean Architecture + ONNX**

```
┌─────────────────────┐
│     TRANSPORT       │  CLI, Future: HTTP/WebSocket
├─────────────────────┤
│    APPLICATION      │  Use Cases, Strategies, Events
├─────────────────────┤
│      DOMAIN         │  Business Logic, Ports
├─────────────────────┤
│  INFRASTRUCTURE     │  ONNX, Video, Storage, Alerts
└─────────────────────┘
```

### **🎛️ Dual Mode**
- **Realtime** : Webcam surveillance, latence < 50ms, alertes immédiates
- **Batch** : Fichiers vidéo, précision maximale, indexation complète

### **🧠 IA Stack - ONNX First**
1. **ONNX Go natif** (5-20ms) - Premier choix
2. **Python embedded** (10-50ms) - Fallback
3. **REST API** (100ms+) - Dernier recours

---

## 📁 **Structure Projet**

```

```

---

## ⚡ **Composants Clés**

### **🎥 Pipeline Vidéo**
- **Sources** : Webcam (gocv), fichiers vidéo, streams RTMP
- **Processing** : Frame extraction, buffering, preprocessing
- **Performance** : Backpressure, worker pools, circuit breakers

### **🧠 IA Engine Agnostique**
```go
type AIProvider interface {
    EncodeText(text string) (Embedding, error)
    EncodeImage(image []byte) (Embedding, error)
    GetLatency() time.Duration
}
```

### **🎯 Semantic Matching**
- **Filtres** : Langage naturel ("person walking", "red car")
- **Matching** : Cosine similarity embeddings
- **Contexte** : Security vs Creative (seuils différents)

### **🚨 Alerting Agnostique**
```go
type AlertSender interface {
    Send(alert Alert) error
    SupportsFormat(format AlertFormat) bool
}
```

---

## 🎮 **Modes d'utilisation**

### **Mode Realtime (Surveillance)**
```bash
livesemantic realtime \
  --source="cam0" \
  --filter="person walking,vehicle entering" \
  --threshold=0.7 \
  --alert="console,webhook" \
  --latency-target=50ms
```

**Optimisations :**
- FPS réduit (5 FPS)
- Résolution adaptée (720p)
- Seuils ajustés sécurité
- Cache embeddings court
- Alertes immédiates

### **Mode Batch (Analyse)**
```bash
livesemantic batch \
  --file="video.mp4" \
  --filters="mariée sourit,applaudissements" \
  --output="highlights/" \
  --export-clips \
  --quality=high
```

**Optimisations :**
- FPS max (précision)
- Full résolution
- Traitement parallèle
- Cache embeddings long
- Indexation complète

---

## 🔧 **Patterns Architecturaux**

### **Strategy Pattern** - Processing par mode
```go
type ProcessingStrategy interface {
    ProcessFrame(frame Frame, filters []Filter) ([]Match, error)
    GetOptimalBatchSize() int
    GetFrameRate() int
}
```

### **Circuit Breaker** - Résilience IA
```go
type LatencyOptimizedAI struct {
    primary   AIProvider  // ONNX rapide
    fallback  AIProvider  // Backup
    circuit   CircuitBreaker
    timeout   time.Duration
}
```

### **Event-Driven** - Découplage composants
```go
type DomainEvent interface {
    AggregateID() string
    OccurredAt() time.Time
    EventType() string
}
```

---

## 📊 **Métriques & Observabilité**

### **Performance Tracking**
- Latence processing par frame
- Throughput (frames/sec, heures video/heure)
- Taux de matches, faux positifs
- Santé des providers IA

### **Agnostique Implementation**
```go
type MetricsCollector interface {
    RecordLatency(operation string, duration time.Duration)
    RecordCounter(metric string, value int64)
    RecordGauge(metric string, value float64)
}
```

**Implémentations :** Console → Prometheus → Cloud metrics

---

## 🚀 **MVP Roadmap**

### **Phase 1 - Foundation** ⭐
- [x] Architecture Clean + ports/adapters
- [ ] ONNX CLIP intégration Go natif
- [ ] Pipeline webcam basique (gocv)
- [ ] CLI realtime surveillance
- [ ] Métriques console

### **Phase 2 - Performance**
- [ ] Cache embeddings LRU
- [ ] Multi-provider AI (ONNX + fallbacks)
- [ ] Backpressure pipeline
- [ ] Mode batch fichiers vidéo

### **Phase 3 - Production**
- [ ] Persistance state (snapshots → DB)
- [ ] API REST + WebSocket
- [ ] Interface web monitoring
- [ ] Containerisation Docker

### **Phase 4 - Scale**
- [ ] Multi-instance deployment
- [ ] Cloud adapters (AWS/GCP)
- [ ] Advanced AI models
- [ ] Distributed processing

---

## 🎯 **Avantages Architecture**

✅ **Performance** : ONNX natif Go, 5-20ms latence  
✅ **Agnostique** : Providers IA, storage, alerting pluggables  
✅ **Résilient** : Circuit breakers, fallbacks multiples  
✅ **Évolutif** : Clean Architecture, event-driven  
✅ **Déployable** : Single binary → multi-cloud  
✅ **Testable** : Ports/adapters, mocking facile  

---

## 🤔 **Prêt pour implémentation MVP ?**

Focus immédiat :
1. **Setup ONNX models** (export Python → Go)
2. **Core domain** (Video, Match, Filter)
3. **ONNX provider** Go natif  
4. **Webcam pipeline** gocv
5. **CLI realtime** avec alertes console

**On démarre par quelle partie ?**