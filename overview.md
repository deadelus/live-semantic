# 🎯 **LiveSemantic - Architecture & Projet**

> Ce document reflète l'état réel du code sur `feat/displayer` (branche la plus avancée) au 2026-08-04. Pour la vision cible et le plan détaillé, voir `TODO.md` et `MIGRATION.md`.

## 📋 **Vue d'ensemble**

**Vision** : analyseur sémantique vidéo temps réel avec filtres IA en langage naturel.

**Réalité actuelle** : détecteur d'objets temps réel à vocabulaire fermé (80 classes COCO, YOLO11s en ONNX natif Go), avec capture webcam, overlay des bounding boxes et CLI. Le matching sémantique en langage naturel (CLIP, embeddings, cosine similarity) n'est **pas implémenté** — les "filtres" actuels comparent une chaîne à un label de classe YOLO, rien de plus.

---

## 🏗️ **Architecture réelle**

```
┌─────────────────────┐
│     TRANSPORT       │  CLI (cobra) ✅, mode interactif ✅ — Web API (gin) et WebSocket : squelettes non branchés
├─────────────────────┤
│    DOMAIN (uc)       │  UseCases.RecognitionUseCase ✅ — un seul use case
├─────────────────────┤
│  INFRASTRUCTURE      │  ports (interfaces) ai.AI, streamer.InputStream/OutputStream, notifier.Notifier ✅
├─────────────────────┤
│  IMPLEMENTATION      │  yolo11s (ONNX natif) ✅, camera gocv ✅, window gocv ✅, log-notifier ✅
└─────────────────────┘
```

L'inversion de dépendance est réelle : `domain/uc` ne dépend que d'interfaces (`infrastructure/*`), les implémentations concrètes vivent dans `implementation/` et sont injectées dans `main.go`. Point faible : `infrastructure/ai.DetectionResult` expose directement le type `onnx.BoundingBox` de la lib vendorisée au lieu d'un type domaine — fuite mineure d'abstraction.

### **🧠 IA Stack — état réel**
1. **ONNX Go natif (YOLO11s)** ✅ — seul backend implémenté, détection d'objets classique (bounding boxes + label + confidence)
2. **CLIP / embeddings texte-image** ❌ — non implémenté, aucun encodeur sémantique
3. **Python embedded / REST API** ❌ — jamais implémenté, resteront des options de fallback futures

---

## 📁 **Structure Projet réelle**

```
src/
├── main.go                        # bootstrap, wiring des dépendances
├── domain/
│   ├── uc/                        # UseCases.RecognitionUseCase (seul use case)
│   ├── dto/                       # DTOs requête/réponse
│   └── model/                     # Frame, Class (constantes COCO), couleurs de box
├── infrastructure/                 # ports (interfaces) : ai.AI, streamer.{Input,Output}Stream, notifier.Notifier
├── implementation/
│   ├── ai/yolo11s/                # adapter ONNX YOLO11s
│   ├── streamer/{input,output}/   # capture webcam gocv, fenêtre d'affichage gocv
│   └── notifier/log-notifier/     # notifier console
├── internal/drawer/                # dessin des bounding boxes sur l'image
└── transport/
    ├── cli/, cmd/                  # CLI cobra + mode interactif — branchés sur RecognitionUseCase ✅
    ├── api/                        # Web API gin — squelette, ne route PAS vers RecognitionUseCase ❌
    └── websocket/                  # squelette, ne route PAS vers RecognitionUseCase ❌
```

~2000 lignes de Go (hors vendor), 2 fichiers de test (`domain/dto`, `domain/uc` — ce dernier sans test réel), couverture quasi nulle.

---

## 🎮 **Modes d'utilisation — état réel**

### **Mode Realtime (webcam)** ✅ implémenté
```bash
./livesemantic recognition --filter="person" --threshold=0.7
```
Capture webcam → YOLO11s → dessin des boxes → fenêtre d'affichage. Fonctionne de bout en bout (testé).

### **Mode Batch (fichiers vidéo)** ❌ non implémenté
Aucune lecture de fichier vidéo, aucune indexation, aucun export de clips.

### **Web API / WebSocket** ❌ squelettes
Routes et handlers présents (34-67 lignes chacun) mais ne branchent sur aucun use case.

---

## 🔧 **Patterns architecturaux — état réel**

- **Strategy Pattern**, **Circuit Breaker**, **Event-Driven** : décrits ci-dessous à titre de vision, **aucun n'est implémenté**. Ce sont des exemples de code illustratifs, pas du code du projet.
- Un prototype de pipeline découplé par channel avait été exploré sur la branche `feat/ochestrator` (code de test se terminant par un `panic()` volontaire) — supprimée depuis, son contenu est documenté dans `TODO.md` / décision C pour référence.

```go
// Exemples de patterns visés, non implémentés à ce jour
type ProcessingStrategy interface { ... }
type LatencyOptimizedAI struct { ... }
type DomainEvent interface { ... }
```

---

## 📊 **Métriques & Observabilité — état réel**

Logs structurés zap (console dev / JSON prod) via `go-clean-app/v2`. Aucun `MetricsCollector`, aucune métrique de latence/throughput/taux de match. Purement des logs, pas de métriques.

---

## 🚀 **MVP Roadmap**

### **Phase 1 - Foundation**
- [x] Architecture Clean + ports/adapters
- [x] ONNX Go natif intégré (YOLO11s, pas CLIP — vocabulaire fermé)
- [x] Pipeline webcam basique (gocv)
- [x] CLI realtime surveillance
- [ ] Métriques console (logs seulement, pas de métriques structurées)

### **Phase 2 - Performance**
- [ ] Cache embeddings LRU (pas d'embeddings à ce jour)
- [ ] Multi-provider AI (ONNX + fallbacks)
- [ ] Backpressure pipeline (prototype non mergé sur `feat/ochestrator`)
- [ ] Mode batch fichiers vidéo

### **Phase 3 - Production**
- [ ] Persistance state (snapshots → DB)
- [ ] API REST + WebSocket (squelettes présents, logique métier absente)
- [ ] Interface web monitoring
- [ ] Containerisation Docker

### **Phase 4 - Scale**
- [ ] Multi-instance deployment
- [ ] Cloud adapters (AWS/GCP)
- [ ] Advanced AI models (CLIP, tracking, cascade — voir `TODO.md`)
- [ ] Distributed processing

---

## 🎯 **Prochaines étapes**

Voir `AUDIT.md` pour l'état des lieux détaillé (branches, hygiène, matrice de recouvrement), `TODO.md` pour le backlog actionnable, et `MIGRATION.md` pour le plan de mise en œuvre phasé.
