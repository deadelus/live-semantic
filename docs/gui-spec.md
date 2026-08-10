# Spec GUI desktop + web — LiveSemantic

Document de travail pour cadrer la GUI (desktop + web, API interne, sources
vidéo multiples). Statut : **proposition, rien de ce document n'est
implémenté** — à valider avant de coder quoi que ce soit. Écrit après
vérification directe du code existant (pas de suppositions non marquées
comme telles).

## 0. Décisions actées (cette passe, 2026-08-10)

- Un seul backend Go, deux façades UI : web (React/Vue/etc., au choix du
  design) + desktop (wrapper natif autour de la même UI web, via
  [Wails](https://wails.io/) — nouvelle dépendance, jamais utilisée dans ce
  projet). Pas deux frontends séparés à maintenir.
- Flux navigateur (webcam locale du navigateur → backend) : **WebRTC en
  priorité**, JPEG-over-WebSocket en fallback/v1 simple si WebRTC prend
  trop de temps.
- Sources vidéo : **USB/webcam locale ET caméra IP/RTSP**, toutes les deux
  dès le début (déjà testable avec la cam de l'ordi ; caméras publiques
  RTSP/HTTP prévues pour valider le chemin réseau).
- **Multi-flux simultané** : plusieurs sources actives en même temps,
  affichées via un système d'onglets/mosaïque (façon vidéosurveillance).
  C'est la demande qui a le plus d'impact architectural (§ 1.2) — ce
  n'est pas juste "afficher plusieurs flux", ça touche la concurrence
  autour des sessions ONNX partagées.
- Ceci répond à la Decision H (topologie de déploiement), jusqu'ici
  ouverte dans `docs/adr/inference-runtimes.md`.

---

## 1. Prérequis backend — à faire AVANT toute ligne de GUI

Ordonné par dépendance. Rien ci-dessous n'existe aujourd'hui sauf mention
contraire.

### 1.1 Câblage de l'API (bloquant absolu, effort M)

`internal/transport/adapters/api` (gin) et `internal/transport/adapters/websocket`
(gorilla) existent comme adaptateurs mais **ne sont reliés à aucune
logique métier** — readme.md le dit explicitement, vérifié dans le code :
zéro appel à `UseCase.RecognitionUseCase` depuis ces deux adaptateurs
aujourd'hui. Seul le CLI (`internal/transport/adapters/cli` /
`internal/transport/adapters/cmd`) est câblé.

À faire :
- Endpoints REST (gin) : démarrer/arrêter une session de reconnaissance,
  changer le filtre/seuil en cours de route, CRUD sur la galerie de
  références (§ 1.4), lister les sources vidéo actives.
- Protocole WebSocket (gorilla) : voir § 2, table transport.
- Définir le modèle de "session" : aujourd'hui `RecognitionUseCase` est un
  appel bloquant unique par processus. Il faut le faire tourner en
  plusieurs instances concurrentes adressables par ID (une par onglet/flux,
  § 1.2).

### 1.2 Concurrence multi-flux (bloquant pour le multiplex demandé, effort L — le plus gros morceau)

**Risque confirmé, pas hypothétique** (vérifié dans le code le
2026-08-10) : `yolo11s.Detector` et `clip.Encoder` réutilisent chacun un
tenseur d'entrée et de sortie **fixes** par instance
(`internal/implementation/inference/onnx/yolo11s/yolo11s.go:59-61`,
`internal/implementation/inference/onnx/clip/clip.go:67-73`). Deux appels
concurrents à `AnalyzeFrame`/`EncodeImage` sur la **même instance** depuis
deux flux différents écriraient/liraient le même buffer en même temps —
corruption de données garantie, pas juste un risque théorique.

Le binding `onnxruntime_go` lui-même ne documente aucune garantie de
thread-safety sur `Run()` (vérifié : aucune mention concurrence/thread
dans le code source du module). **Non vérifié empiriquement** — à tester
avec `-race` et un vrai harness multi-goroutines avant de trancher.

Options à évaluer (pas tranché) :
- **Une instance `Detector`/`Encoder` par flux actif** — le plus simple à
  raisonner, coûte de la mémoire (chaque session ONNX charge son propre
  modèle en RAM) et du temps de démarrage par flux. À chiffrer (poids
  mémoire de yolo11s.onnx + les deux CLIP .onnx, multiplié par N flux).
- **Pool de sessions partagées avec file d'attente** — moins de mémoire,
  introduit une latence de contention entre flux (un flux attend qu'une
  session se libère), complexifie le code.
- **Sérialisation totale** (un seul verrou global autour de toute
  inférence) — le plus simple à coder, mais annule le bénéfice du
  multi-flux en cas de charge (un flux lent bloque tous les autres).

Impact CPU à mesurer avant de promettre du "temps réel" multi-caméra :
aucun Execution Provider GPU n'est câblé aujourd'hui (§ 1.6), tout tourne
CPU. Mesuré en session solo (`docs/adr/clip-backend.md` § 8, CPU ARM) :
~182ms YOLO + ~46ms/boîte CLIP par flux. Avec 3-4 flux simultanés en
sérialisé, la latence par flux se multiplie d'autant — à valider avant
d'annoncer un nombre de flux simultanés supporté.

Contention `gocv` existante à revalider en multi-flux : le cache
`sharedFrameMat` (`internal/implementation/tracking/gocv-tracker/tracker.go`)
et le réglage `OPENCV_OPENCL_DEVICE=disabled`/`SetNumThreads(1)`
(`docs/adr/object-tracking.md` § 7-8, `TODO.md` § F) ont été validés pour
**un seul flux** — jamais testés en concurrence réelle (`-race` compilé
et passé en unitaire uniquement, pas en conditions webcam réelles, cf.
TODO.md § C dernier item).

### 1.3 Nouveaux adaptateurs `InputStream`

Le port `streamer.InputStream` existe déjà (`internal/infrastructure/streamer`)
— extension sans toucher domain/application, seulement de nouveaux
adaptateurs dans `internal/implementation/streamer/`.

- **USB/webcam avec device configurable** (effort XS) : aujourd'hui
  `input.NewCameraInput()` ouvre le device `0` en dur
  (`internal/implementation/streamer/input/camera.go`). Paramétrer l'index.
- **RTSP/caméra IP** (effort S, MAIS non testé bout-en-bout) : `gocv`
  expose `VideoCaptureFile(uri)` / `OpenVideoCapture(url)`, et il existe
  une constante `VideoCaptureFFmpeg` dans le binding — le transport RTSP
  est supporté par l'API Go. **Vérifié le 2026-08-10** : le build OpenCV
  local a bien FFmpeg comme dépendance (`brew info opencv`). **Pas encore
  testé avec un vrai flux RTSP** (aucun sous la main au moment d'écrire
  ceci) — à valider avec une caméra IP ou un flux public avant de
  considérer ce point acquis. Gestion de la reconnexion à prévoir (un flux
  réseau peut couper, contrairement à une webcam locale).
- **Frames navigateur (JPEG-over-WS)** (effort S) : le backend reçoit des
  frames JPEG poussées périodiquement sur une connexion WebSocket dédiée,
  les décode en `entities.Frame`. Réutilise l'infra WS déjà prévue pour
  l'affichage (§ 2).
- **WebRTC (navigateur → backend)** (effort L, prioritaire selon la
  décision ci-dessus) : nécessite `pion/webrtc` (jamais utilisé dans ce
  projet, nouvelle dépendance CGo-free en Go pur — contrairement à
  `gocv`/`onnxruntime_go`, c'est un vrai avantage, pas de fragilité CGo
  supplémentaire). Le backend agit comme un pair WebRTC, décode les frames
  vidéo reçues en `entities.Frame`. **Point technique à anticiper** : si
  une "caméra publique" testée par l'utilisateur n'est pas sur le même
  réseau local, la traversée NAT nécessite un serveur STUN (souvent
  suffisant) voire TURN (si NAT symétrique) — pas juste "WebRTC marche
  tout seul" en pair-à-pair direct. À prévoir dans l'estimation d'effort.

### 1.4 Galerie de références (feature métier à construire, effort M)

Décidé dans une passe précédente (`TODO.md` § D), zéro ligne de code à ce
jour. Store en mémoire `{label, embedding}`, alimenté par `EncodeImage`
sur un crop sélectionné en direct (§ 3.2), comparé par similarité cosinus
à chaque détection au lieu du filtre texte (ou en complément — à trancher
dans la spec fonctionnelle, § 3.3).

### 1.5 Sélection runtime + labellisation (feature métier, effort M)

Nécessite un mécanisme événementiel propre côté `UseCase` (pas juste côté
UI) : recevoir "l'utilisateur a sélectionné cette région à cet instant",
en extraire un crop de la frame courante, l'encoder, l'ajouter à la
galerie. À concevoir comme un nouveau cas d'usage ou une extension de
`RecognitionUseCase`, pas bricolé dans le transport.

### 1.6 Execution Provider GPU (optionnel, mais pertinent si charge multi-flux élevée)

`runtime.WithCUDA()`/`WithTensorRT()`/`WithOpenVINO()` existent déjà côté
Go (`internal/implementation/inference/onnx/runtime/options.go`) mais ne
sont câblés nulle part (ni CLI ni ailleurs) — jamais testés sur ce projet.
Si le multi-flux CPU (§ 1.2) s'avère trop lent, c'est le levier naturel,
mais pas un prérequis strict pour un premier multi-flux (2-3 sources).

### 1.7 Wrapper desktop Wails (effort S une fois le web fonctionnel)

Nouvelle dépendance. Embarque la UI web dans une fenêtre native, lance le
serveur backend en local au démarrage. Pas de travail UI dupliqué.

---

## 2. Couche transport

| Flux | Sens | Transport | Format | Priorité | Effort |
|---|---|---|---|---|---|
| Vidéo + boxes + scores → GUI (affichage live) | backend → client | WebSocket | Frames JPEG (ou MJPEG-like) + JSON (boxes, label, score CLIP, track ID) par message | P0 | S |
| Webcam navigateur → backend (pipeline YOLO/CLIP) | client → backend | **WebRTC** (prioritaire) | Flux vidéo décodé côté Go via `pion/webrtc` | P0 | L |
| Webcam navigateur → backend (fallback) | client → backend | WebSocket (JPEG-over-WS) | Frame JPEG poussée à N fps | P1 (fallback si WebRTC bloque) | S |
| Caméra USB/RTSP → backend | — (local au backend) | `gocv`/FFmpeg direct | — (ne repasse jamais par le navigateur) | P0 (USB) / P1 (RTSP, non testé) | XS (USB) / S (RTSP) |
| Contrôle (filtres, seuils, galerie, sélection runtime) | client ↔ backend | REST (gin) pour commandes ponctuelles, WebSocket pour retour live (ex. score qui bouge pendant qu'on bouge un slider) | JSON | P0 | M |
| Multi-flux / sessions | client ↔ backend | Chaque onglet/tuile = une session logique avec un ID, une connexion WS dédiée pour son flux vidéo, ses commandes REST scopées par `sessionID` | — | P0 | Dépend de § 1.2 |

---

## 3. Spec fonctionnelle GUI

### 3.1 Gestion des sources (nouveau — conséquence directe du multi-flux demandé)

- Panel "Sources" : liste des flux configurés (webcam locale, URL RTSP,
  webcam navigateur), ajout/suppression/édition.
- Chaque source = un onglet **ou** une tuile dans une vue grille/mosaïque
  (à trancher avec le design — les deux sont raisonnables, l'onglet est
  plus simple à câbler côté état applicatif, la mosaïque est plus utile
  pour de la surveillance multi-caméra réelle).
- Statut par source : connecté / déconnecté / erreur. Reconnexion
  automatique pour les sources réseau (RTSP, WebRTC) — une webcam locale
  ne devrait normalement pas se déconnecter, un flux réseau si.

### 3.2 Vue live (par flux)

- Flux vidéo avec boxes dessinées dessus, rafraîchi en continu.
- **Sélection runtime d'un objet** : clic sur une box existante, ou
  clic-glisser pour dessiner une box à main levée si rien n'est détecté
  dessus → champ de saisie pour le label → ajout à la galerie de
  références (§ 1.4/1.5). Chaque entrée de galerie : renommable,
  supprimable, activable/désactivable individuellement.
- **Couleur des box configurable** — aujourd'hui codée en dur par classe
  COCO (`internal/domain/entities/class_color.go`, `BoxIDColors`), pas par
  track ni par filtre. À redesigner : une couleur par entrée de
  galerie/filtre actif (cohérent avec le fait que CLIP a remplacé le
  matching par classe COCO, `TODO.md` § A), sélecteur manuel + palette
  auto par défaut.
- Score de similarité affiché en direct par box (`TrackEvent.Score`,
  ajouté au backend le 2026-08-10 — déjà disponible, juste jamais
  visualisé).
- Overlay perf optionnel : FPS, latence YOLO/CLIP, nb de tracks actifs
  (déjà loggé en JSON via `zap`, jamais affiché dans une UI).

### 3.3 Panel de contrôle / filtres

**Point à trancher, pas encore décidé** : les filtres sont-ils **par
flux** (chaque caméra cherche autre chose — logique pour un usage
surveillance multi-caméra) ou **globaux** (même filtre partout) ?
Recommandation : par flux, avec un raccourci "appliquer à toutes les
sources actives" pour l'usage simple à une seule caméra.

- Champ filtre texte libre — multi-filtres simultanés par flux (le
  backend n'accepte qu'une seule string aujourd'hui, `dto.RecognitionRequest.Filter`
  — à faire évoluer vers une liste, § 1.1).
- Slider de seuil de similarité **avec retour visuel en direct** (pas un
  champ numérique aveugle) : le calibrage est instable et dépend de la
  classe (`docs/adr/clip-backend.md` § 7-9, marges mesurées de 0.01-0.03
  seulement) — voir le score bouger en live pendant qu'on ajuste vaut
  mieux qu'une valeur par défaut statique.
- Toggle par entrée de galerie (activer/désactiver sans supprimer).
- Sélecteur tracker KCF/CSRT (existe en dur, jamais exposé).
- Sélecteur de source vidéo par onglet.

### 3.4 Réglages avancés (par flux, avec un défaut global overridable)

Tous ces paramètres existent en dur dans le code aujourd'hui, aucun n'est
exposé :

| Paramètre | Valeur actuelle | Fichier |
|---|---|---|
| Downscale tracking | 320px | `gocv-tracker/tracker.go: maxTrackingDimension` |
| Seuil association IoU | 0.3 | `tracking.go: iouAssociationThreshold` |
| Hits avant confirmation | 3 | `track.go: minHitsToConfirm` |
| Misses avant disparition | 2 | `track.go: maxMissesBeforeLost` |
| Anti-spam alertes | 5s | `tracking.go: notifyDebounce` |
| Execution provider ONNX | CPU only | `runtime/options.go`, jamais exposé (§ 1.6) |
| Modèle YOLO / CLIP | yolo11s / CLIP quantized uniquement | pas de choix de variante |

### 3.5 Historique / alertes

- Log agrégé multi-flux des événements (`TrackEntered`/`TrackMatched`/
  `TrackLost`), avec timestamp, source, label, score CLIP.
- Filtre "quelle caméra" sur l'historique.
- Export clip vidéo / capture sur match — n'existe pas, zéro ligne de
  code, à concevoir si demandé.

---

## 4. Risques / inconnues à valider empiriquement avant de s'engager

Par ordre d'impact sur le planning :

1. **Thread-safety des sessions ONNX en concurrence** (§ 1.2) — bloquant
   pour tout multi-flux réel, pas testé, risque confirmé par lecture de
   code (tenseurs fixes réutilisés).
2. **Charge CPU réelle en multi-flux** — aucun EP GPU câblé, tout CPU.
   Latence par flux mesurée en solo (~180-320ms/cycle reanchor) à
   multiplier par le facteur de contention choisi en § 1.2.
3. **RTSP réel non testé** — FFmpeg confirmé présent dans le build, mais
   aucun flux RTSP testé bout-en-bout à ce jour.
4. **WebRTC + NAT traversal** pour des sources non locales (caméras
   publiques) — STUN/TURN à prévoir, pas juste "WebRTC marche tout seul".
5. **`gocv` en concurrence réelle** — `OPENCV_OPENCL_DEVICE=disabled`,
   `SetNumThreads(1)`, `sharedFrameMat` validés pour un flux unique
   uniquement (`docs/adr/object-tracking.md`), jamais en charge
   multi-flux réelle.

---

## 5. Ordre d'implémentation recommandé

1. Câblage API minimal (gin + WS) sur le flux existant (webcam locale
   unique) — valide le bout-en-bout sans la complexité multi-flux.
2. Device USB configurable + RTSP (extension `InputStream`), test avec
   une vraie caméra IP/flux public.
3. Concurrence multi-flux (§ 1.2) — le plus gros morceau technique,
   décision pool/instance-par-flux/sérialisation après mesure, pas avant.
4. Ingestion WebRTC navigateur.
5. Fallback JPEG-over-WS (peut être fait en parallèle du point 4, plus
   simple).
6. Wrapper desktop Wails.
7. Galerie de références + sélection runtime (§ 1.4/1.5).
8. UI complète (couleurs configurables, historique, réglages avancés).
