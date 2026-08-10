# Spec GUI desktop + web — LiveSemantic

Document de travail pour cadrer la GUI (desktop + web, API interne, sources
vidéo multiples). Statut : **proposition, rien de ce document n'est
implémenté** — à valider avant de coder quoi que ce soit. Écrit après
vérification directe du code existant (pas de suppositions non marquées
comme telles).

Pour le brief produit/design à transmettre (écrans, interactions, décisions
UX — sans les détails techniques backend ci-dessous), voir
`docs/gui-design-brief.md`.

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

### 1.2 Concurrence multi-flux (bloquant pour le multiplex demandé) — **investigué et résolu le 2026-08-10**

**Risque confirmé par un test réel** (outil jetable
`cmd/onnx-concurrency-test`, supprimé après usage, compilé avec `-race`,
2 runs) : `yolo11s.Detector` (et par le même motif `clip.Encoder`)
réutilisent chacun un tenseur d'entrée/sortie **fixes** par instance
(`yolo11s.go:59-61`, `clip.go:67-73`). Appeler `AnalyzeFrame` concurremment
sur la **même instance** depuis 8 goroutines a produit **3 data races
confirmées par le détecteur Go** (écritures concurrentes dans
`writeInput()` sur le buffer du tenseur d'entrée partagé) — pas une
supposition, un résultat de `go build -race` reproduit 2/2.

**Solution trouvée, testée, et confirmée correcte** : le binding
`onnxruntime_go` expose aussi `ort.DynamicAdvancedSession`
(`onnxruntime_go.go:2197`, disponible dans la v1.21.0 utilisée par ce
projet), dont `Run(inputs, outputs []Value)` prend les tenseurs **en
paramètre à chaque appel** au lieu de les figer à la création — exactement
le contrat de thread-safety documenté par l'API C d'ONNX Runtime
([microsoft/onnxruntime#114](https://github.com/microsoft/onnxruntime/issues/114) :
`Run()` est thread-safe sur une session partagée tant que chaque appel
utilise ses propres buffers `OrtValue`, pas les mêmes). Testé : **8
goroutines × 5 appels concurrents sur une session `DynamicAdvancedSession`
partagée**, tenseurs d'entrée frais par goroutine/appel, entrée identique
partout → **0 divergence de sortie** entre goroutines (2/2 runs sans
`-race`, cohérent avec `-race` qui ne signale aucune race sur cette partie
du test). Contrairement à ce qui était supposé plus tôt (§ options ci-avant,
maintenant obsolètes), **pas besoin d'une instance/session par flux** — le
modèle (poids ONNX) reste chargé une seule fois et partagé, seuls les
tenseurs (petits pour CLIP texte/image, ~7.5 Mo cumulés input+output pour
YOLO à 640×640) sont alloués par appel.

**Décision retenue : migrer `yolo11s.Detector` et `clip.Encoder` de
`AdvancedSession` vers `DynamicAdvancedSession`**, avec allocation de
tenseurs frais par appel plutôt que des tenseurs figés en champs de
struct. Reste à faire (pas fait dans cette investigation, qui visait à
trancher l'approche, pas à livrer le code) :
- Réécrire `yolo11s.go`/`clip.go` sur ce nouveau pattern.
- Mesurer le coût réel de l'allocation par appel (non benchmarké) — non
  bloquant pour trancher l'architecture, mais à surveiller : si le coût
  GC devient significatif sous charge multi-flux, un pool de tenseurs
  (`sync.Pool` par goroutine/flux) resterait possible sans revenir aux
  tenseurs figés par session.
- Revalider `gocv-tracker` séparément (§ ci-dessous, pas concerné par
  cette investigation — c'est un risque distinct, pas résolu ici).

**Point annexe trouvé pendant le test, à documenter pour la suite** : le
binaire compilé avec `-race` **plante systématiquement à la sortie**
(`libc++abi: ... mutex lock failed: Invalid argument`, SIGABRT — 2/2
runs), alors que le même code **sans** `-race` sort proprement (2/2 runs).
Ressemble à une interaction connue entre l'instrumentation du race
detector Go et le nettoyage natif (CGo) d'ONNX Runtime — distinct du bug
SIGABRT déjà corrigé (`runtime.DestroyEnvironment()`, celui-là avait un
message d'erreur différent). **Pas un bug de production** (les binaires
livrés ne sont jamais compilés avec `-race`), mais un piège pour de
futurs tests de concurrence sur ce projet : valider la détection de races
avec `-race`, puis valider la propreté de sortie **sans** `-race`
séparément — ne pas s'étonner d'un crash-à-la-sortie sous `-race` et le
prendre pour un vrai bug.

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
- **Fichier vidéo local** (effort XS, **déjà prouvé dans ce projet**) :
  `gocv.VideoCaptureFile(path)` sur un fichier local est déjà utilisé et
  fonctionnel — `cmd/tracking-drift/main.go:96`, le test de dérive
  KCF/CSRT tourne dessus depuis `TODO.md` § B. Il manque juste un
  adaptateur `InputStream` propre (aujourd'hui c'est un outil jetable
  headless, pas un port), pas de risque technique résiduel.
- **Flux live YouTube (ou similaire)** (effort S, nouvelle dépendance
  externe) : une URL YouTube n'est pas directement lisible par
  FFmpeg/OpenCV — il faut d'abord résoudre l'URL réelle du flux média
  (HLS/DASH) via un outil comme [yt-dlp](https://github.com/yt-dlp/yt-dlp)
  (`yt-dlp -g <url>` renvoie une URL directe, ensuite ouvrable par
  `gocv.VideoCaptureFile`/FFmpeg comme n'importe quel flux HTTP). **Ce
  n'est pas une dépendance Go** — c'est un binaire externe à shell-out
  (ou une lib équivalente si une existe en Go, non cherché). Implication :
  le backend doit soit embarquer/exiger `yt-dlp` sur la machine, soit
  gérer son absence proprement. Youtube change régulièrement son
  fonctionnement interne (obfuscation, throttling) — `yt-dlp` est
  maintenu activement justement pour ça, mais c'est un point de
  fragilité externe au projet à surveiller (pas un choix "install once,
  works forever"). Non testé à ce jour.

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
| Vidéo + boxes + scores → GUI, onglet actif (vue complète) | backend → client | WebSocket | Frames JPEG (ou MJPEG-like) + JSON (boxes, label, score CLIP, track ID) par message, framerate normal | P0 | S |
| Vidéo → GUI, tuiles mosaïque (aperçu léger, § 3.1) | backend → client | WebSocket | Frames JPEG seules, ~1 fps, **sans** boxes/JSON — un flux distinct/dégradé du précédent, pas le même abonnement | P1 (dépend du choix mosaïque, § 3.1) | S, mais protocole à concevoir en même temps que la ligne au-dessus (pas après) |
| Webcam navigateur → backend (pipeline YOLO/CLIP) | client → backend | **WebRTC** (prioritaire) | Flux vidéo décodé côté Go via `pion/webrtc` | P0 | L |
| Webcam navigateur → backend (fallback) | client → backend | WebSocket (JPEG-over-WS) | Frame JPEG poussée à N fps | P1 (fallback si WebRTC bloque) | S |
| Caméra USB/RTSP/fichier local → backend | — (local au backend) | `gocv`/FFmpeg direct | — (ne repasse jamais par le navigateur) | P0 (USB, fichier local) / P1 (RTSP, non testé) | XS (USB, fichier local — déjà prouvé) / S (RTSP) |
| Flux YouTube/live externe → backend | — (local au backend) | `yt-dlp` (shell-out) résout l'URL réelle, puis `gocv`/FFmpeg direct | — | P1 | S (dépendance externe, non testée) |
| Contrôle (filtres, seuils, galerie, sélection runtime) | client ↔ backend | REST (gin) pour commandes ponctuelles, WebSocket pour retour live (ex. score qui bouge pendant qu'on bouge un slider) | JSON | P0 | M |
| Multi-flux / sessions | client ↔ backend | Chaque onglet/tuile = une session logique avec un ID, une connexion WS dédiée pour son flux vidéo, ses commandes REST scopées par `sessionID` | — | P0 | Dépend de § 1.2 |

---

## 3. Spec fonctionnelle GUI

**Décidé le 2026-08-10** : thème clair/sombre au choix de l'utilisateur
(toggle accessible, pas enfoui) — les deux variantes doivent être conçues,
pas seulement un sombre par défaut.

### 3.1 Gestion des sources (nouveau — conséquence directe du multi-flux demandé)

- Panel "Sources" : liste des flux configurés (webcam locale, URL RTSP,
  webcam navigateur, fichier vidéo, flux YouTube/live), ajout/suppression/
  édition.
- **Décidé le 2026-08-10** : deux modes d'affichage au choix de
  l'utilisateur (toggle), pas un seul mode imposé :
  - **Vue liste** : compacte, textuelle.
  - **Vue mosaïque** : grille de tuiles vidéo, chacune affichant un
    **aperçu léger** (~1 fps, **sans boxes ni overlay** — juste l'image
    brute) pour rester fluide même avec beaucoup de flux actifs. Cliquer
    sur une tuile bascule automatiquement vers la **vue onglets** et
    active l'onglet de ce flux, qui affiche lui la vidéo complète
    (framerate normal, boxes, scores, contrôles). La vue onglets devient
    la vue active par défaut après un clic depuis la mosaïque ;
    retour manuel à la mosaïque possible ensuite.
- **Implication transport (§ 2)** : ça veut dire deux qualités de flux
  distinctes à servir depuis le backend, pas une seule — un flux "preview"
  léger (1 fps, pas de boxes, pour toutes les tuiles de la mosaïque en
  simultané) et un flux "complet" (framerate normal, boxes+score, pour
  l'onglet actif uniquement). Pas encore reflété dans la table § 2 —
  à faire quand le protocole WS sera conçu en détail (§ 1.1).
- Statut par source : connecté / déconnecté / erreur / reconnexion en
  cours — visible aussi bien en vue liste qu'en vue mosaïque (badge sur la
  tuile / sur la ligne). Reconnexion automatique pour les sources réseau
  (RTSP, WebRTC, YouTube) — une webcam locale ne devrait normalement pas
  se déconnecter, un flux réseau si.

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

1. ~~Thread-safety des sessions ONNX en concurrence~~ — **résolu le
   2026-08-10** (§ 1.2) : testé avec `-race`, confirmé unsafe dans le
   pattern actuel, fix identifié et validé (`DynamicAdvancedSession`).
   Reste à *implémenter* le fix dans `yolo11s.go`/`clip.go` (pas fait,
   l'investigation visait à trancher l'architecture).
2. **Charge CPU réelle en multi-flux** — aucun EP GPU câblé, tout CPU.
   Latence par flux mesurée en solo (~180-320ms/cycle reanchor) à
   multiplier par le facteur de contention choisi en § 1.2.
3. **RTSP réel non testé** — FFmpeg confirmé présent dans le build, mais
   aucun flux RTSP testé bout-en-bout à ce jour.
4. **WebRTC + NAT traversal** pour des sources non locales (caméras
   publiques) — STUN/TURN à prévoir, pas juste "WebRTC marche tout seul".
5. **Dépendance externe `yt-dlp`** pour les flux YouTube — binaire non-Go
   à shell-out, à installer/vérifier sur la machine cible, sujet à casser
   quand YouTube change son fonctionnement interne (maintenu activement
   pour ça, mais reste un point de fragilité hors du contrôle du projet).
6. **`gocv` en concurrence réelle** — `OPENCV_OPENCL_DEVICE=disabled`,
   `SetNumThreads(1)`, `sharedFrameMat` validés pour un flux unique
   uniquement (`docs/adr/object-tracking.md`), jamais en charge
   multi-flux réelle. Distinct de § 1.2 (ONNX) — pas couvert par cette
   investigation, toujours ouvert.

---

## 5. Ordre d'implémentation recommandé

1. Câblage API minimal (gin + WS) sur le flux existant (webcam locale
   unique) — valide le bout-en-bout sans la complexité multi-flux.
2. Migration `yolo11s.Detector`/`clip.Encoder` vers `DynamicAdvancedSession`
   (§ 1.2, architecture tranchée, reste à coder) — prérequis réel du
   multi-flux, plus petit qu'estimé initialement (pas de pool/sérialisation
   à construire, juste changer le pattern d'allocation des tenseurs).
3. Device USB configurable + RTSP + fichier vidéo local (extension
   `InputStream`), test avec une vraie caméra IP/flux public.
4. Multi-flux (plusieurs `RecognitionUseCase` concurrents, adressables par
   ID) — s'appuie sur le point 2, mesurer la charge CPU réelle (§ 4.2)
   avant d'annoncer un nombre de flux simultanés supporté.
5. Ingestion WebRTC navigateur.
6. Fallback JPEG-over-WS (peut être fait en parallèle du point 5, plus
   simple) + flux YouTube (`yt-dlp`, indépendant du reste).
7. Wrapper desktop Wails.
8. Galerie de références + sélection runtime (§ 1.4/1.5).
9. UI complète (couleurs configurables, historique, réglages avancés).
