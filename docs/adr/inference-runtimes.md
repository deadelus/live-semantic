# Runtimes & formats d'inférence — état de l'art et décision

> **Statut** : référence technique + ADR
> **Portée** : choix du format de modèle et du runtime d'inférence pour LiveSemantic
> **Dernière révision** : 2026-08

---

## 1. Clarification de vocabulaire

| Terme | Nature | Rôle |
|---|---|---|
| **TensorFlow** | Framework complet | Définir, entraîner, exporter, servir |
| **PyTorch** | Framework complet | Idem, standard de fait en recherche |
| **ONNX** | Format de fichier | Graphe de calcul sérialisé (protobuf), opérateurs standardisés versionnés par *opset*. **N'exécute rien.** |
| **ONNX Runtime (ORT)** | Runtime | Exécute un `.onnx`. Projet distinct d'ONNX. |
| **TensorRT / OpenVINO / LiteRT** | Compilateurs + runtimes | Optimisent pour une cible matérielle précise |

Le vrai axe de comparaison est donc :
**framework d'entraînement** → **format d'échange** → **runtime d'inférence**.

---

## 2. Le pipeline réel

```
Entraînement  →  Export / IR   →  Compilation  →  Runtime  →  Serving
─────────────    ────────────      ───────────     ───────    ────────
PyTorch          ONNX              TensorRT        ORT        Triton
TensorFlow       StableHLO         XLA             LiteRT     TF Serving
JAX              ExecuTorch        TVM / IREE      CoreML     vLLM
```

La majorité des équipes entraînent aujourd'hui en **PyTorch** (~90 % de la
recherche), puis exportent vers ONNX ou TensorRT pour la production.
TensorFlow reste présent principalement via son écosystème de déploiement
historique (TFX, TF Serving, TFLite) et dans les organisations qui ont investi
dessus il y a 5 à 8 ans.

---

## 3. Subtilités opérationnelles

Ces quatre points sont les sources de friction les plus fréquentes en
production. Les connaître à l'avance évite des jours de debug.

### 3.1 Les opsets ONNX

Chaque opérateur ONNX a un numéro de version. Un modèle exporté en **opset 18**
ne tournera pas sur un runtime qui plafonne à **opset 15**.

→ **Règle projet** : figer l'opset à l'export, le documenter, et le vérifier au
chargement du modèle.

### 3.2 Couverture d'opérateurs à l'export

L'export ONNX casse typiquement sur :

- le contrôle de flux dynamique (boucles, branches dépendant des données),
- les opérateurs custom,
- les shapes variables mal annotées.

`torch.onnx.export` en mode **dynamo** (le nouveau chemin) est nettement plus
robuste que l'ancien tracing. Prévoir malgré tout une passe de simplification du
graphe via `onnx-simplifier`.

### 3.3 Les Execution Providers (EP) d'ORT

ORT ne calcule pas lui-même : il délègue à un backend — CPU, CUDA, TensorRT,
DirectML, CoreML, OpenVINO, QNN.

⚠️ **Piège majeur** : si un opérateur n'est pas supporté par l'EP choisi, ORT
retombe silencieusement sur CPU pour ce nœud. On se retrouve avec des
allers-retours mémoire GPU↔CPU qui détruisent la latence, **sans aucun message
d'erreur**.

→ **Règle projet** : profiler, ne pas croire. Activer le profiling ORT et
vérifier le placement effectif des nœuds.

### 3.4 Quantification

ONNX supporte INT8/INT4 via des nœuds QDQ, mais la qualité du résultat dépend
fortement du backend. TensorRT effectue sa propre calibration et ignore souvent
ce qui est encodé dans le fichier.

→ Toujours mesurer la dégradation de précision **sur le backend cible**, pas en
théorie.

---

## 4. Matrice de décision

| Situation | Choix recommandé |
|---|---|
| Modèle vision/NLP classique servi depuis un backend non-Python | **ONNX + ORT** |
| GPU NVIDIA, latence critique, dernier pourcent de perf | **TensorRT** |
| CPU Intel / edge x86 | **OpenVINO** |
| Mobile Android / iOS | **LiteRT** (ex-TFLite), **ORT Mobile** ou **CoreML** |
| LLM en production serveur | **vLLM** / **SGLang** / **TensorRT-LLM** — pas ONNX |
| LLM en local sur machine perso | **llama.cpp** + format **GGUF** |
| Écosystème Google déjà en place | **TensorFlow + TF Serving** |
| Multi-modèles, multi-frameworks, un seul serveur | **Triton Inference Server** |

---

## 5. Backends matériels : TensorRT / OpenVINO / LiteRT

Ces trois occupent la même case du pipeline — **compilateur + runtime spécialisé
pour un matériel** — chacun adossé à un fondeur.

| | TensorRT | OpenVINO | LiteRT |
|---|---|---|---|
| **Éditeur** | NVIDIA | Intel | Google |
| **Cible** | GPU NVIDIA uniquement | CPU x86, iGPU, NPU Intel | Mobile, embarqué, navigateur |
| **Artefact** | `.engine` / `.plan` | IR `.xml` + `.bin` | `.tflite` (FlatBuffers) |
| **Compilation** | AOT, longue (minutes) | Rapide | Conversion rapide |
| **Artefact portable** | ❌ lié au GPU + version | ✅ | ✅ |
| **Pertinence LiveSemantic** | Haute si GPU NVIDIA | Haute si serveur CPU Intel | Nulle (pas de backend Go) |

### TensorRT

Le plus agressif des trois. Il ne se contente pas d'exécuter : il **recompile**
le graphe pour le GPU cible — fusion de couches, sélection de kernels par
benchmark réel, calibration INT8/FP8 propriétaire.

Contrepartie : l'engine produit est lié au modèle de GPU, à la version de
TensorRT **et** au driver. Changement de machine ⇒ régénération obligatoire.

→ **Conséquence projet** : c'est une étape de *build au déploiement*, pas un
artefact à versionner dans le repo. Compter plusieurs minutes de compilation
pour un modèle vision.

Choix pertinent quand la latence est le critère n°1 et que le parc matériel est
maîtrisé.

### OpenVINO

Souvent sous-estimé. Sur **CPU x86**, il dépasse régulièrement ORT en
configuration par défaut d'un facteur 2 à 3, en exploitant finement AVX-512 et
AMX. Il lit l'ONNX directement, sans conversion préalable obligatoire.

Couvre aussi les iGPU Intel et les NPU des Core Ultra. Fonctionne sur AMD, mais
sans optimisation dédiée.

Choix pertinent pour un déploiement serveur **sans GPU** — cas très courant en
surveillance installée sur site.

### LiteRT

Nouveau nom de TensorFlow Lite depuis septembre 2024. Même runtime, même format
`.tflite`, mais l'appellation ne reflétait plus la réalité : il accepte
désormais des modèles PyTorch, JAX et Keras. L'écosystème s'est élargi à
LiteRT-LM (LLM embarqués) et LiteRT.js (navigateur, via WebAssembly/WebGPU).

⚠️ **Hors périmètre LiveSemantic.** Cible Android/iOS/MCU/navigateur, pas de
binding Go sérieux. À connaître pour ne pas s'y perdre, pas à intégrer.

### Le pattern qui compte : backend = configuration, pas code

Ces trois s'utilisent de deux façons : **en direct** via leur propre API, ou
comme **Execution Provider d'ORT**.

La seconde valide la décision d'architecture (§7). Le code Go appelle toujours
`onnxruntime_go` ; seule la liste d'EP change, via un `*ort.SessionOptions`
construit **avant** la création de la session — rien d'autre ne bouge, ni le
decode, ni l'appelant :

```go
sessionOptions, err := ort.NewSessionOptions()
// ...

// Serveur CPU Intel
sessionOptions.AppendExecutionProviderOpenVINO(map[string]string{"device_type": "CPU"})

// Serveur GPU NVIDIA
trtOpts, err := ort.NewTensorRTProviderOptions()
sessionOptions.AppendExecutionProviderTensorRT(trtOpts)

// Au lieu du `nil` passé aujourd'hui :
session, err := ort.NewAdvancedSession(modelPath, inputNames, outputNames, inputs, outputs, sessionOptions)
```

Le domaine ne bouge pas. L'adapter `ObjectDetector` ne bouge pas. On passe d'un
déploiement CPU à un déploiement GPU **par configuration**.

Coût : quelques pourcents de perf perdus face à un TensorRT utilisé en direct.
Gain : un seul adapter à maintenir au lieu de trois. Pour le MVP, l'arbitrage
est tranché.

⚠️ Rappel du piège §3.3 : avec l'EP TensorRT, les opérateurs non supportés
retombent **silencieusement** sur CPU. Sans profiling, on peut croire tourner
sur GPU alors que la moitié du graphe est sur processeur.

**État actuel du code** (2026-08-05) : `internal/implementation/inference/onnx/yolo11s.go`
passe encore `nil` en dernier argument de `ort.NewAdvancedSession` — CPU par
défaut, aucun EP configurable. `internal/implementation/inference/onnx/runtime/`
(aujourd'hui : `LibraryPath()` + `InitEnvironment()`) est l'endroit naturel où
faire atterrir la construction du `*ort.SessionOptions` le jour où un EP est
réellement nécessaire — voir item backlog ci-dessous.

---

## 6. Écosystème élargi (comparables utiles)

### PyTorch / ExecuTorch
Standard de fait pour l'entraînement. **ExecuTorch** est la réponse de Meta à
TFLite pour l'embarqué — sans passer par ONNX.

### JAX / XLA / StableHLO
Approche « compilateur d'abord ». JAX est du NumPy différentiable compilé via
XLA. **StableHLO** est l'IR portable qui joue, côté Google, le rôle qu'ONNX joue
côté PyTorch. C'est ce qui tourne sur TPU.

### TVM et IREE
Compilateurs ML qui **autotunent** le code généré pour une cible matérielle
spécifique. Mise en œuvre plus lourde, mais imbattables sur du hardware
exotique.

### GGUF + llama.cpp
Écosystème parallèle des LLM quantifiés. Aucun rapport avec ONNX, format
totalement à part. C'est ce qui permet de faire tourner un modèle 7B sur un
laptop.

### Triton Inference Server (NVIDIA)
Serveur d'inférence qui avale simultanément ONNX, TensorRT, PyTorch et TF, avec
batching dynamique.

> ⚠️ **Ne pas confondre** avec le langage **Triton d'OpenAI**, qui sert à écrire
> des kernels GPU. Homonymie malheureuse et fréquente source de confusion en
> revue de code.

### BentoML / KServe / Ray Serve
Couche au-dessus : packaging, autoscaling, déploiement Kubernetes.

---

## 7. ADR — Décision pour LiveSemantic

### Contexte

LiveSemantic est un binaire **Go** qui analyse des flux vidéo en temps réel avec
des filtres sémantiques en langage naturel (embeddings type CLIP). Contraintes :

- latence cible sub-50 ms par frame,
- distribution sous forme de binaire unique autant que possible,
- pas de dépendance à un runtime Python en production.

### Analyse — l'angle Go

C'est l'argument décisif.

**TensorFlow en Go est écarté** : les bindings officiels sont abandonnés et
nécessitent `libtensorflow` via CGo — une dépendance lourde et pénible à
distribuer.

**ONNX Runtime** expose une **API C propre et stable**. Le binding
[`github.com/yalue/onnxruntime_go`](https://github.com/yalue/onnxruntime_go)
fonctionne bien : on charge le `.so`/`.dll`, on passe des tenseurs, c'est tout.

Alternative sans CGo du tout : exposer ORT derrière **Triton** en gRPC. Souvent
le choix le plus sain pour un backend Go en production, au prix d'un hop réseau.

### Décision

**Chemin canonique retenu** :

```
Entraînement / modèle pré-entraîné (PyTorch)
        ↓  torch.onnx.export (mode dynamo, opset figé)
    modèle .onnx  +  onnx-simplifier
        ↓
ONNX Runtime via onnxruntime_go   ← MVP, mono-binaire
        ou
Triton Inference Server (gRPC)    ← si scaling multi-modèles
```

### Conséquences

**Positives**
- Un seul format de modèle à gérer dans le repo.
- Portabilité CPU/GPU sans changer le code Go, uniquement l'EP.
- Découplage net : la couche `infrastructure/ai` ne dépend que d'une interface.

**Négatives / à surveiller**
- Dépendance CGo au binding ORT → impact sur la cross-compilation.
- Nécessité de versionner l'opset et la version d'ORT ensemble.
- Le fallback CPU silencieux des EP impose un profiling systématique.

### Points de vigilance à intégrer au backlog

État au 2026-08-06 :

- [x] Figer et documenter l'opset ONNX utilisé à l'export — extrait du protobuf du modèle : **opset 19**, IR version 9, exporté depuis PyTorch 2.7.0. Documenté en commentaire sur `yolo11sModelPath` dans `internal/implementation/inference/onnx/yolo11s.go`.
- [x] Valider la version d'ORT au démarrage, échouer explicitement si incompatible — `runtime.RequireMinVersion()` (`internal/implementation/inference/onnx/runtime/version.go`, testé), appelé dans `yolo11s.New()`. Minimum fixé à 1.20.0 ; libs bundlées actuelles = 1.22.0.
- [ ] Activer le profiling ORT en mode debug pour vérifier le placement des nœuds — **bloqué** : le binding `onnxruntime_go` v1.21.0 n'expose aucune API de profiling (pas de `EnableProfiling`/équivalent trouvé dans le binding). Pour débloquer : soit contribuer cette fonctionnalité au binding en amont, soit l'ajouter via cgo direct dans le projet — pas fait, effort non trivial pour un gain qui n'est utile qu'une fois un vrai doute de placement de nœuds se présente (aujourd'hui : CPU only, la question ne se pose pas encore).
- [ ] Benchmarker la quantification INT8 sur le backend cible réel — **bloqué** : aucun modèle quantifié n'existe dans `assets/models/` (seul `yolo11s.onnx` en float32). Préalable côté export Python (PyTorch/onnx quantization), pas un travail côté `live-semantic`.
- [x] Exposer la liste d'Execution Providers en configuration, jamais en dur — `runtime.Option` (`internal/implementation/inference/onnx/runtime/options.go`) : `WithCUDA()`, `WithTensorRT()`, `WithOpenVINO(deviceType)`. `yolo11s.New(opts ...runtime.Option)` les accepte, défaut CPU inchangé.
- [ ] Benchmarker ORT-CPU vs OpenVINO sur la cible de déploiement réelle — **bloqué** : vérifié par `strings` sur `assets/libraries/osx/onnxruntime_arm64.dylib`, le plugin `libonnxruntime_providers_openvino.dylib` n'est pas bundlé (l'EP échouerait au chargement avec "Failed to load shared library"). CUDA/TensorRT présents dans le binaire mais inutilisables sans GPU NVIDIA (ce Mac n'en a pas). Pour débloquer : obtenir une distribution ORT avec le plugin OpenVINO inclus, ou la compiler soi-même.
- [x] Documenter la procédure de cross-compilation avec CGo — `docs/development/cross-compilation.md`. Testé réellement (pas supposé) : `GOOS=linux GOARCH=amd64 go build` échoue dès le runtime cgo (headers macOS incompatibles avec la cible Linux), avant même d'atteindre gocv/OpenCV. Exemple trompeur dans `readme.md` corrigé en conséquence.

---

## 8. Références

- ONNX — spécification et opsets : https://onnx.ai
- ONNX Runtime — Execution Providers : https://onnxruntime.ai/docs/execution-providers/
- Binding Go : https://github.com/yalue/onnxruntime_go
- Triton Inference Server : https://github.com/triton-inference-server/server