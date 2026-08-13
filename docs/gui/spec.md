# Spec GUI desktop + web — LiveSemantic

Document de travail pour cadrer la GUI (desktop + web, API interne, sources
vidéo multiples). Statut au **2026-08-13** : la majorité de § 1 (prérequis
backend) est **faite et testée** — câblage API, multi-flux
(`session.Manager`), galerie REST, ingestion navigateur JPEG-over-WS,
protocole `/ws` structuré. **Le frontend `web/` a été basculé le même
jour** de l'ancienne API mono-session (`/api/v1/recognition/*`, `/ws`)
vers les endpoints multi-flux (`/api/v1/sessions/*`,
`/ws/sessions/:id[/ingest]`, voir `web/src/api.ts`) — un seul
session/onglet pour l'instant (la vue liste/mosaïque multi-source, § 3.1,
n'existe pas encore), mais chaque appel est déjà scopé par session, donc
additif pour la suite plutôt qu'une nouvelle migration à refaire. La
galerie de références n'a en revanche pas encore d'UI (§ 1.4).

Pour le brief produit/design à transmettre (écrans, interactions, décisions
UX — sans les détails techniques backend ci-dessous), voir
`docs/gui/design-brief.md`.

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

### 1.1 Câblage de l'API — **fait le 2026-08-10, étendu au multi-flux le 2026-08-12**

~~`internal/transport/adapters/api` (gin) et `internal/transport/adapters/websocket`
(gorilla) existent comme adaptateurs mais ne sont reliés à aucune logique
métier~~ — **obsolète, plus vrai depuis le 2026-08-10.** Les deux serveurs
gin séparés ont été fusionnés en un seul (`internal/transport/adapters/api`,
flag `-s`/`--web`), réellement branché sur `uc.Recognition`/
`uc.GalleryReferences`. Testé bout-en-bout en conditions réelles (webcam
locale + session fichier simultanées, curl/client WS, `go test -race`
propre sur `internal/...`).

Ce qui existe aujourd'hui côté backend :
- **Mono-session (legacy, toujours actif)** : `POST/POST/GET
  /api/v1/recognition/{start,stop,status}`, `GET /ws` (vidéo), `GET
  /ws/ingest` (caméra navigateur).
- **Multi-flux (`session.Manager`, depuis le 2026-08-12)** : `POST/GET
  /api/v1/sessions`, `GET/DELETE /api/v1/sessions/:id`, `POST
  /api/v1/sessions/:id/recognition/{start,stop}`, `GET
  /ws/sessions/:id`, `GET /ws/sessions/:id/ingest` — additif, ne remplace
  pas le mono-session.
- **Galerie de références** : `POST/GET/PATCH/DELETE /api/v1/gallery`
  (§ 1.4, backend fait, UI pas commencée).

**Ce qui n'est pas fait** : le frontend `web/` ne consomme que le chemin
mono-session — la bascule vers `/api/v1/sessions/*` + `/ws/sessions/:id`
est un chantier séparé, pas commencé (voir l'accroche en tête de fichier).

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

**Décision mise en œuvre : `yolo11s.Detector` et `clip.Encoder` migrés de
`AdvancedSession` vers `DynamicAdvancedSession` — fait le 2026-08-11.**
Tenseurs input/output désormais alloués par appel
(`ort.NewEmptyTensor` + `defer .Destroy()` locaux dans
`AnalyzeFrame`/`EncodeImage`/`EncodeText`), plus de champs de struct
partagés. **Le `session.Manager` multi-flux (§ 1.1, fait le 2026-08-12)
s'appuie directement sur cette migration** — plusieurs sessions partagent
`objectDetector`/`semanticEncoder` sans instance dédiée par flux, comme
prévu ici. Testé en conditions réelles (webcam + fichier vidéo
simultanément, filtres différents, `go test -race` propre) mais **pas de
mesure de charge CPU à N flux réels** — toujours ouvert, voir § 4 point 2.
Reste non fait :
- Coût de l'allocation par appel toujours pas benchmarké isolément (pas
  de régression observée manuellement, pas mesuré précisément).
- `gocv-tracker` en concurrence réelle (§ 4 point 6) — distinct de ce
  risque ONNX, toujours pas revalidé en charge multi-flux.

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

- **USB/webcam avec device configurable** — **fait le 2026-08-11** :
  `input.NewCameraInput(device int)`, plus de `0` en dur. Pas encore
  exposé via CLI/REST/GUI (source selection, § H1 Multi-flux/H2) — juste
  rendu possible côté adaptateur.
- **RTSP/caméra IP** — **testé bout-en-bout le 2026-08-11** via
  `cmd/rtsp-smoke-test` (outil dev conservé dans le repo, pas jetable —
  pas de test automatisé versionné possible pour RTSP) contre un flux
  auto-hébergé (`mediamtx` + `ffmpeg` en boucle) : dimensions correctes,
  cleanup propre, débit variable (~18-32 fps une fois la session
  stabilisée) sur plusieurs runs. **Reconnexion automatique sur coupure
  réseau toujours pas implémentée** (lacune documentée, pas un oubli).
- **Frames navigateur (JPEG-over-WS)** — **fait le 2026-08-12** :
  `implementation/streamer/input.BrowserInput` + `GET /ws/ingest` (legacy)
  et son pendant par session `GET /ws/sessions/:id/ingest` ; côté
  frontend, `web/src/BrowserCamera.ts` pointe désormais sur ce dernier
  (bascule du 2026-08-13, voir accroche en tête de fichier).
- **WebRTC (navigateur → backend)** (effort L, toujours pas commencé,
  prioritaire selon la décision ci-dessus une fois attaqué) : nécessite
  `pion/webrtc` (jamais utilisé dans ce
  projet, nouvelle dépendance CGo-free en Go pur — contrairement à
  `gocv`/`onnxruntime_go`, c'est un vrai avantage, pas de fragilité CGo
  supplémentaire). Le backend agit comme un pair WebRTC, décode les frames
  vidéo reçues en `entities.Frame`. **Point technique à anticiper** : si
  une "caméra publique" testée par l'utilisateur n'est pas sur le même
  réseau local, la traversée NAT nécessite un serveur STUN (souvent
  suffisant) voire TURN (si NAT symétrique) — pas juste "WebRTC marche
  tout seul" en pair-à-pair direct. À prévoir dans l'estimation d'effort.
- **Fichier vidéo local** — **fait le 2026-08-11** :
  `implementation/streamer/input.FileInput` (même type que RTSP/caméra
  IP ci-dessus — `gocv.VideoCaptureFile(uri)` résout les deux depuis le
  schéma de l'URI, pas deux adaptateurs séparés), testé contre
  `assets/videos/car.mp4` (238 frames, cleanup propre), 8 tests
  table-driven versionnés.
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

### 1.4 Galerie de références / Bibliothèque — **backend (v1 plate) fait le 2026-08-12, UI pas commencée, modèle produit revu depuis (mockups 4a-4e)**

Store en mémoire `{label, embedding}` (`implementation/gallery/inmemory.Gallery`,
port `infrastructure/gallery.Repository`), comparé par similarité cosinus
à chaque détection en complément du filtre texte. REST complet et testé
en conditions réelles : `POST /api/v1/gallery` (upload multipart réel),
`GET`/`PATCH` (rename/enable-disable)/`DELETE`. **Reste ouvert : l'UI de
sélection en direct côté H2** (clic sur une box existante ou dessin à
main levée → mini-formulaire label → appel `POST /api/v1/gallery` avec le
crop, § 1.5/3.2) — dépend du scaffolding H2 existant, pas commencée.

**Écart modèle à noter avant de construire l'UI** (design figé dans les
mockups du 2026-08-13, `docs/gui/design-brief.md` § Bibliothèque) : le
produit attendu est plus riche que le store plat actuel —
**Collection** (nom + tags plats) › **Terme** (nom + **1..N photos
obligatoires**, chacune un vecteur image lié au vecteur texte du nom ; un
Terme sans photo n'existe pas) + **classe COCO liée optionnelle**
(restreint l'évaluation aux boîtes déjà classées YOLO dans cette classe).
Un Terme peut appartenir à plusieurs Collections sans duplication. Rien de
tout ça (Collections, tags, multi-photo par entrée, lien COCO explicite)
n'existe côté `gallery.Repository`/`inmemory.Gallery` aujourd'hui — à
chiffrer comme extension du modèle de données backend avant d'attaquer
l'UI H2, pas juste un habillage visuel du store existant.

### 1.5 Sélection runtime + labellisation (feature métier, effort M)

Nécessite un mécanisme événementiel propre côté `UseCase` (pas juste côté
UI) : recevoir "l'utilisateur a sélectionné cette région à cet instant",
en extraire un crop de la frame courante, l'encoder, l'ajouter à la
galerie. À concevoir comme un nouveau cas d'usage ou une extension de
`RecognitionUseCase`, pas bricolé dans le transport.

### 1.5bis Pause/reprise + buffer de rewind sur la Vue live (ajouté 2026-08-10, feature métier, effort M)

Demandé dans `docs/gui/design-brief.md` : pause/reprise de la lecture côté
client, et retour en arrière sur les N dernières secondes/minutes (durée
configurable par l'utilisateur), sans couper le flux réel qui continue
tourner en arrière-plan (la détection/tracking ne doit pas se mettre en
pause juste parce que l'utilisateur regarde en arrière).

C'est le même besoin technique que le "V2 ring buffer des ~15 dernières
frames" déjà noté dans `TODO.md` § C (à l'origine pensé pour laisser le
tracker rattraper un décalage, pas pour un usage utilisateur) — même
mécanisme, deux usages différents, à concevoir ensemble plutôt que deux
fois séparément.

Coût réel à chiffrer avant de promettre une durée par défaut : un tampon
glissant de N secondes × FPS × taille de frame, par flux actif. Pour une
frame JPEG compressée (raisonnable pour du replay, pas besoin du flux brut
non compressé) à 640×480 environ ~30-80 Ko/frame selon la compression ; à
15 fps sur 30 secondes ça fait déjà ~15-35 Mo par flux, multiplié par le
nombre de flux actifs en multi-caméra (§ 1.2) — pas négligeable si
plusieurs flux gardent chacun un long historique. Non mesuré, à valider
avant de fixer une durée par défaut dans l'UI.

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

**Fait le 2026-08-12** : la ligne "vue complète" ci-dessous est implémentée
avec un protocole en deux messages par frame (`streamer.BoxAwareOutputStream`) —
un message binaire (JPEG **non annoté**) + un message JSON texte séparé
(`{"boxes":[...]}`, coordonnées normalisées `[0,1]`, toujours envoyé même
vide), pas les boxes déjà dessinées dans le JPEG comme le premier jet du
2026-08-10. Existe en mono-session (`/ws`) et par session (`/ws/sessions/:id`).
Reste non fait : la ligne "tuiles mosaïque" (abonnement par flux avec
fps/boxes ajustables, § 3.1) — aucun protocole dédié n'existe encore.

| Flux | Sens | Transport | Format | Priorité | Effort |
|---|---|---|---|---|---|
| Vidéo + boxes + scores → GUI, onglet actif (vue complète) | backend → client | WebSocket | Frames JPEG (ou MJPEG-like) + JSON (boxes, label, score CLIP, track ID) par message, framerate normal | P0 — **fait** | S |
| Vidéo → GUI, tuiles mosaïque (aperçu léger, § 3.1) | backend → client | WebSocket | Frames JPEG seules, ~1 fps, **sans** boxes/JSON — un flux distinct/dégradé du précédent, pas le même abonnement | P1 (dépend du choix mosaïque, § 3.1) | S, mais protocole à concevoir en même temps que la ligne au-dessus (pas après) |
| Webcam navigateur → backend (pipeline YOLO/CLIP) | client → backend | **WebRTC** (prioritaire) | Flux vidéo décodé côté Go via `pion/webrtc` | P0 | L |
| Webcam navigateur → backend (fallback) | client → backend | WebSocket (JPEG-over-WS) | Frame JPEG poussée à N fps | P1 (fallback si WebRTC bloque) — **fait, backend + frontend mono-session** | S |
| Caméra USB/RTSP/fichier local → backend | — (local au backend) | `gocv`/FFmpeg direct | — (ne repasse jamais par le navigateur) | P0 (USB, fichier local) / P1 (RTSP) — **tous faits et testés côté adaptateur**, pas encore sélectionnables depuis la GUI | XS (USB, fichier local) / S (RTSP) |
| Flux YouTube/live externe → backend | — (local au backend) | `yt-dlp` (shell-out) résout l'URL réelle, puis `gocv`/FFmpeg direct | — | P1 | S (dépendance externe, non testée) |
| Contrôle (filtres, seuils, galerie, sélection runtime) | client ↔ backend | REST (gin) pour commandes ponctuelles, WebSocket pour retour live (ex. score qui bouge pendant qu'on bouge un slider) | JSON | P0 — REST galerie/recognition **fait**, seuil/sélection runtime pas exposés | M |
| Multi-flux / sessions | client ↔ backend | Chaque onglet/tuile = une session logique avec un ID, une connexion WS dédiée pour son flux vidéo, ses commandes REST scopées par `sessionID` | — | P0 — **backend fait (`session.Manager`), frontend pas branché dessus** | Dépend de § 1.2 |

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
- **Volet de droite de l'accueil (repliable)** : réglages de la mosaïque
  elle-même — afficher ou non les boxes sur les tuiles, augmenter leur FPS
  d'affichage (ex. surveiller 4 caméras à la fois avec plus de détail),
  avec avertissement de latence selon le matériel côté serveur (charge
  CPU, § 1.2/1.6) affiché dans l'UI, pas caché.
- **Implication transport (§ 2), revue le 2026-08-10** : `docs/gui/design-brief.md`
  précise que la qualité de la mosaïque doit être **réglable par
  l'utilisateur** (FPS plus élevé + boxes visibles sur les tuiles, ex.
  pour surveiller 4 caméras à la fois), pas juste un mode "preview" fixe à
  1 fps. Ça veut dire que le protocole WS ne peut pas être conçu comme
  deux tiers figés (preview vs complet) — il faut un abonnement par flux
  avec des paramètres ajustables (fps cible, inclure ou non les boxes),
  la vue onglet n'étant qu'un cas particulier avec les paramètres au
  maximum. Revoir la table § 2 dans ce sens avant implémentation. Latence
  attendue et dépendante du matériel côté serveur (charge CPU, § 1.2/1.6)
  si plusieurs tuiles montent leur FPS en simultané — l'UI doit exposer
  ce compromis à l'utilisateur, pas le cacher.
- Statut par source : connecté / déconnecté / erreur / reconnexion en
  cours — visible aussi bien en vue liste qu'en vue mosaïque (badge sur la
  tuile / sur la ligne). Reconnexion automatique pour les sources réseau
  (RTSP, WebRTC, YouTube) — une webcam locale ne devrait normalement pas
  se déconnecter, un flux réseau si.

### 3.2 Onglet Vue live (ouvert au clic sur une source depuis § 3.1)

**Pas un écran indépendant** — c'est ce qui s'affiche dans l'onglet ouvert
au clic sur une tuile/ligne de § 3.1. §§ 3.2-3.4 ci-dessous décrivent tous
le contenu de ce même onglet (vidéo + volet de droite repliable :
boxes/filtres/avancés), pas trois écrans séparés — structure clarifiée le
2026-08-10 suite à `docs/gui/design-brief.md` (qui avait la même ambiguïté
au départ, corrigée là-bas en premier).

- Flux vidéo avec boxes dessinées dessus, rafraîchi en continu.
- **Lecture/pause/reprise + retour en arrière (rewind)**, durée de buffer
  configurable — voir § 1.5bis pour l'implication backend (tampon glissant
  de frames par flux, coût mémoire non chiffré).
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

### 3.3 Volet de droite de l'onglet — filtres

**Décidé** (suite à `docs/gui/design-brief.md`, plus "à trancher") : les
filtres sont **par flux**, avec un raccourci "appliquer à toutes les
sources actives" pour l'usage simple à une seule caméra — pas un choix
global unique imposé.

> ⚠️ **Mis à jour le 2026-08-11 (après-midi) — le backend est redevenu
> hybride, ce qui rouvre (partiellement) le slider mais change sa portée.**
> Trouvé plus tôt le même jour : le backend avait abandonné CLIP pour un
> filtrage exact par label COCO (`docs/adr/clip-backend.md` § 12). Ça a été
> révisé quelques heures après (§ 13) : **un terme classe COCO matche
> exactement (pas de score, pas de slider possible/utile)** ; **un terme
> texte libre matche via CLIP** contre un seuil (toujours fragile, §7/§10,
> pour l'instant une constante cachée côté backend, pas exposée
> CLI/API). Implication GUI, pas encore construite :
> - Le champ filtre doit couvrir les deux cas : un **sélecteur de labels
>   COCO** (match exact, pas de slider) **et** un **champ texte libre**
>   (match sémantique, avec un slider de seuil qui redevient pertinent —
>   mais seulement pour les termes texte libre, pas les labels).
> - Le plafond (`*N`) a aussi deux sens différents à refléter dans l'UI :
>   pour un label, c'est un plafond simple (premier arrivé) ; pour du texte
>   libre, c'est un **top-N par score** (les N meilleurs candidats, pas les
>   N premiers détectés) — probablement pas la même UI pour les deux.
> - Le seuil sémantique n'est toujours pas un paramètre de requête
>   aujourd'hui (constante `defaultSimilarityThreshold`, backend) — il faut
>   décider si/quand cette constante devient un vrai paramètre exposé côté
>   API avant de construire un slider qui n'aurait rien à piloter à
>   distance.

- Champ filtre — **hybride, avec priorité d'autocomplétion fixée par les
  mockups du 2026-08-13** (écran 4e, voir `docs/gui/design-brief.md` §
  Bibliothèque) : une combobox unique propose, dans l'ordre, (1) Classes
  COCO natives du modèle actif (match exact), (2) Termes de "Ma
  Bibliothèque" (§ 1.4 — comparaison au(x) vecteur(s) image du Terme,
  restreinte à sa classe COCO liée si présente), puis (3) texte libre en
  dernier recours seulement si rien ne correspond aux deux premiers
  groupes (option grisée "mode moins fiable"). Le backend n'accepte
  qu'une seule string aujourd'hui (`dto.RecognitionRequest.Filter`, spec
  `"label*N,texte libre*N"`) et ne connaît pas la notion de Terme de
  Bibliothèque distincte d'un label COCO ou d'un texte libre — à faire
  évoluer vers une structure plus riche côté transport (§ 1.1) une fois
  le modèle Collections/Termes construit côté backend (§ 1.4).
- Slider de seuil de similarité avec retour visuel en direct — **redevient
  pertinent, mais seulement pour les termes texte libre** (pas les labels
  COCO, qui n'ont pas de score). Le calibrage reste instable et dépend de
  la classe (`docs/adr/clip-backend.md` § 7-10, marges de 0.01-0.03) — voir
  le score bouger en live pendant qu'on ajuste vaut mieux qu'une valeur par
  défaut statique. Nécessite d'abord que le seuil devienne un vrai
  paramètre de requête côté backend (voir encart ci-dessus).
- **Décidé le 2026-08-11** : case à cocher **par terme de filtre** pour
  autoriser le chevauchement (`overlap`, TODO.md § A, pas construit côté
  backend — juste la syntaxe/le paramètre nommé, pas encore d'implémentation
  à piloter). Par défaut décoché : un terme exact et un terme sémantique
  visant le même objet physique ne dessinent jamais deux boxes superposées
  (confirmé par l'utilisateur, testé en réel — seul devant la caméra,
  aucune double box voulue). Cochée, la GUI devra indiquer clairement que
  deux entrées de galerie/deux conditions peuvent se déclencher sur le même
  objet — pas un défaut silencieux.
- Toggle par entrée de galerie (activer/désactiver sans supprimer).
- Sélecteur tracker KCF/CSRT (existe en dur, jamais exposé).

### 3.4 Volet de droite de l'onglet — section "avancés" (repliée par défaut)

Sous-section dépliable du même volet de droite que § 3.3, pas un écran ni
un panel séparé. Tous ces paramètres existent en dur dans le code
aujourd'hui, aucun n'est exposé :

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
- **Point ouvert côté design** (`docs/gui/design-brief.md`) : accessible
  depuis l'accueil (§ 3.1) ou depuis chaque onglet (§ 3.2), pas encore
  tranché — pas d'implication backend particulière dans un sens ou
  l'autre (le log est agrégé multi-flux quoi qu'il arrive).

---

## 4. Risques / inconnues à valider empiriquement avant de s'engager

Par ordre d'impact sur le planning :

1. ~~Thread-safety des sessions ONNX en concurrence~~ — **résolu et
   implémenté** : `DynamicAdvancedSession` en place depuis le 2026-08-11,
   `session.Manager` multi-flux dessus depuis le 2026-08-12, testé en
   conditions réelles (webcam + fichier simultanés, `-race` propre).
2. **Charge CPU réelle en multi-flux** — toujours ouvert, aucun EP GPU câblé, tout CPU.
   Latence par flux mesurée en solo (~180-320ms/cycle reanchor) à
   multiplier par le facteur de contention choisi en § 1.2.
3. ~~RTSP réel non testé~~ — **résolu le 2026-08-11** :
   `implementation/streamer/input.FileInput` testé bout-en-bout via
   `cmd/rtsp-smoke-test` (outil dev conservé, pas jetable — pas de test
   automatisé possible pour RTSP) contre un flux auto-hébergé (`mediamtx` +
   `ffmpeg` en boucle sur un asset du repo, aucun flux public fiable
   disponible) — dimensions correctes, cleanup propre, zéro erreur sur
   plusieurs runs ; débit variable d'un run à l'autre contre un publisher
   fraîchement démarré (quelques frames à ~18-32 fps une fois la session
   stabilisée). La reconnexion automatique sur coupure réseau reste non
   implémentée (lacune documentée, pas testée).
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

État au 2026-08-13 — points 1 à 4 et 6 sont **faits** côté backend, et le
frontend `web/` a été basculé sur le multi-flux le même jour (§ 1.1). La
suite (5, 7, 8 UI, 9) reste à faire — voir la section H2 du backlog de
développement pour le détail écran par écran.

1. ~~Câblage API minimal (gin + WS)~~ — **fait le 2026-08-10.**
2. ~~Migration `DynamicAdvancedSession`~~ — **fait le 2026-08-11.**
3. ~~Device USB configurable + RTSP + fichier vidéo local~~ — **fait et
   testé le 2026-08-11** (RTSP via `mediamtx` auto-hébergé, pas de flux
   public fiable disponible).
4. ~~Multi-flux (`session.Manager`)~~ — **fait le 2026-08-12, frontend
   basculé le 2026-08-13.** Charge CPU réelle à N flux toujours pas
   mesurée (§ 4 point 2).
5. Ingestion WebRTC navigateur — **pas commencé.**
6. ~~Fallback JPEG-over-WS~~ — **fait le 2026-08-12** (backend), frontend
   basculé sur l'ingestion par session le 2026-08-13. Flux YouTube
   (`yt-dlp`) — **pas commencé**, indépendant du reste.
7. Wrapper desktop Wails — **pas commencé**, attend un H2 fonctionnel.
8. ~~Galerie de références~~ — **backend fait le 2026-08-12**, UI et
   sélection runtime (§ 1.5) **pas commencées**.
9. UI complète (couleurs configurables, historique, réglages avancés) —
   **pas commencée**.
