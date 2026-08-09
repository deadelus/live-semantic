# TODO — LiveSemantic

Backlog dérivé des décisions d'architecture validées (voir contexte de mission). Chaque tâche référence sa ligne dans `AUDIT.md` § Étape 2bis. Les tâches marquées **[DÉJÀ FAIT → À INTÉGRER]** ne sont pas à recoder : le travail existe mais est non commité (stash) ou non mergé (branche) — le premier geste est de le récupérer, pas de le réécrire.

Ordre de dépendance global : **G (hygiène) → E (ports) → B (IoU/tracker) → D (Track) → C (async) → A (cascade) → F (latences, en continu)**. H reste une question ouverte, pas une tâche.

---

## 🔴 Bug critique — SIGABRT à la sortie dès que ORT + gocv coexistent dans le process

Découvert le 2026-08-09 en construisant l'outil de test de dérive (`cmd/tracking-drift`), **hors de tout scope prévu** — pas un artefact de cet outil, reproduit à la racine.

**Reproduction minimale** (3 variantes testées, toutes crashent identiquement) :
```go
detector, _ := yolo11s.New()
detector.Cleanup()
_ = gocv.NewMat() // ou VideoCaptureFile(...), ou même juste `import _ "gocv.io/x/gocv"`
// → SIGABRT à la sortie du process, systématique
```
`libc++abi: terminating due to uncaught exception ... mutex lock failed: Invalid argument`. Reproduit **indépendamment de l'ordre** (ORT avant ou après gocv), **indépendamment de l'usage réel** (un import aveugle de gocv suffit, aucun appel n'est nécessaire), et **indépendamment de la vidéo/caméra** (touche `gocv.NewMat()` seul, sans `VideoCapture`). Tout pointe vers un conflit de destructeurs statiques C++ entre les deux libs natives vendorisées (`onnxruntime_go` et `gocv`), pas un bug de logique métier.

**Impact réel : très probablement présent dans le binaire `livesemantic` de production.** `main.go` instancie systématiquement `yolo11s.New()` (ORT) et `input.NewCameraInput()`/`Initialize()` (gocv) dans le même process pour toute commande `recognition` — le crash devrait donc se reproduire à la sortie normale (Escape/Ctrl+C) de n'importe quelle session `livesemantic recognition` réelle. **Non confirmé sur le binaire réel** (pas de webcam/écran dans cet environnement pour le vérifier de bout en bout), mais la réduction minimale ci-dessus ne laisse pas beaucoup de place au doute.

- [ ] Confirmer sur une machine avec webcam/écran que `livesemantic recognition` crashe bien au `Escape`/Ctrl+C. **Premier essai réel le 2026-08-09 : `echo $?` = 0, pas de SIGABRT observé sur ce run précis** — soit non-déterministe, soit la réduction minimale (probe isolé) ne reproduit pas exactement les conditions réelles (ordre d'init, nombre d'allocations gocv, etc.). Pas encore résolu, à re-tester sur plusieurs runs avant de conclure dans un sens ou l'autre.
- [ ] Root-cause : isoler si c'est `onnxruntime_go`, `gocv`, ou l'interaction des deux qui enregistre le destructeur fautif (`atos`/`lldb` sur le core, ou bissection par retrait de fonctionnalités des deux libs). **Effort : M, debugging CGo natif — pas un simple correctif Go.**
- [ ] Corriger ou contourner (candidats à évaluer, aucun validé) : ordre d'init différent, `os.Exit` explicite avant retour de `main` (probablement inefficace, les handlers `atexit` C tournent quand même), issue upstream sur l'un des deux repos. **Dépendance : root-cause ci-dessus. Effort : inconnu tant que la cause n'est pas isolée.**

## G — Restructuration & hygiène (bloquant, faible risque, à faire en premier)

Référence matrice : G — restructuration absente partout, problèmes d'hygiène confirmés en étape 2. **Phase terminée le 2026-08-05** (voir `MIGRATION.md` § Phase 1 pour le détail de ce qui a réellement été fait, la structure finale diffère légèrement de la proposition initiale — affinée avec l'utilisateur avant exécution).

- [x] `git rm --cached .env`, créer `.env.example` avec les 4 clés sans valeurs sensibles. `.env` et `.DS_Store` ajoutés à `.gitignore`.
- [x] Ajouter `LICENSE` (MIT).
- [x] Remplacer les placeholders `github.com/your-org/livesemantic` par `github.com/deadelus/live-semantic` dans `readme.md`.
- [x] Corriger les promesses de latence non étayées ("sub-50ms") dans `readme.md`.
- [ ] Purger les `*.onnx` de l'**historique** Git (réécriture via `git filter-repo`/BFG), sans passer par Git LFS pour l'instant — décision : on ignore la piste LFS/téléchargement-au-build pour le moment, on flush directement. **Pas fait** — ils sont déjà regroupés proprement dans `assets/` (fait) pour l'état courant, mais l'historique Git existant n'a pas été réécrit. **Attention avant exécution** : un `filter-repo --invert-paths` naïf supprimerait aussi les `.onnx` de l'état courant (aucun mécanisme de téléchargement au build n'existe encore) — il faudra une réécriture qui préserve le blob dans le commit le plus récent, ou un remplacement par un stub + téléchargement, sinon le build casse. Le poids du `.git` (136 Mo, `MIGRATION.md` § Risques) s'est encore alourdi : le stash `yoloe11s-seg.onnx` (41 Mo) récupéré sur `recover/yoloe11s-seg-stash` doit être inclus dans le périmètre de la purge.
- [x] Migration `src/` → `cmd/livesemantic/` + `internal/`. Structure finale (affinée avec l'utilisateur, différente de la proposition initiale `ports/`+`infrastructure/`) :
  `internal/domain/entities` (pur, ex-`model/`, renommé) · `internal/application/{uc,dto}` (orchestre domaine + ports) · `internal/infrastructure/*` (interfaces/ports, nom conservé du code existant) · `internal/implementation/*` (adapters concrets, nom conservé, `drawer/` y a rejoint) · `internal/transport/{handlers,envelopes,adapters/{api,cli,cmd,websocket}}` (raffiné a posteriori, voir `MIGRATION.md`) · `cmd/livesemantic/main.go`. Assets binaires (fonts, modèles ONNX, libs natives) sortis du code source vers `assets/` à la racine, chemins hardcodés mis à jour en conséquence. Build/vet/test/smoke-test tous verts après coup.
- [x] Déplacer les DTO vers la couche applicative — fait (`internal/application/dto`), en même temps que la migration ci-dessus plutôt qu'en deux passes.

## E — Ports (inversion de dépendance complète)

Référence matrice : E — **Phase terminée le 2026-08-06.** Les 5 ports cibles existent (`ObjectDetector`, `ObjectTracker`, `SemanticEncoder`, `AlertSender`, `MetricsCollector`) avec la forme voulue (pas de fuite vendor). `ObjectTracker`/`SemanticEncoder`/`MetricsCollector` sont des interfaces sans implémentation câblée — normal, ils dépendent respectivement de B, A, et de points d'instrumentation qui n'existent pas encore.

- [x] Renommer `infrastructure/ai.AI` en `infrastructure/inference.ObjectDetector` (fait le 2026-08-05, avant même le reste de la phase E — l'occasion s'est présentée en réorganisant `implementation/ai` → `implementation/inference/onnx/{runtime,yolo11s}`).
- [x] Corriger la fuite d'abstraction — **fait de facto le 2026-08-05, effet de bord du commit `4cad31d`** (remplacement de `go-clean-onnxruntime` par `onnxruntime_go`) : `decodeDetections` (`yolo11s.go`) construit directement `[]entities.BoundingBox` (`Label`, `Confidence`, `X1/Y1/X2/Y2`), plus aucune trace de `onnx.BoundingBox` dans le code (vérifié par grep le 2026-08-06). `DetectionResult.BoundingBoxes []entities.BoundingBox` est déjà un type domaine propre. Item non coché jusqu'ici par oubli de mise à jour de cette TODO après le refacto — pas un travail restant.
- [x] Créer `ObjectTracker` (`internal/infrastructure/tracking/tracker.go`) : `Init(frame, box) error` / `Update(frame) (entities.BoundingBox, bool)`. Interface seule, non implémentée (**dépend du choix du tracker gocv, voir B**), non câblée dans `main.go`/`UseCase` — rien à instancier tant qu'aucun adapter n'existe.
- [x] Créer `SemanticEncoder` (`internal/infrastructure/inference/semantic_encoder.go`) : `EncodeImage(frame)`/`EncodeText(text)` → `entities.Embedding` (nouveau type, `internal/domain/entities/embedding.go`, `[]float32`). Interface seule, non implémentée (**dépend du backend CLIP, voir A**), non câblée.
- [x] `AlertSender` : `notifier.Notifier` renommé `notifier.AlertSender` (`internal/infrastructure/notifier/notification.go`). Contrat inchangé (`Notify(msg entities.Message) error` + `Cleanup()`) — il correspondait déjà à la cible, pas d'adaptation nécessaire. `LogNotifier` et tous les appelants (`uc.UseCase`, `main.go`) mis à jour.
- [x] Créer `MetricsCollector` (`internal/infrastructure/metrics/metrics.go`) : `RecordLatency(stage, duration)` / `IncrementCounter(name)`. Implémentation console fournie (`internal/implementation/metrics/console-metrics`, `ConsoleMetrics`, logge via `logger.Logger` injecté). **Non câblée dans `UseCase`/`main.go`** : aucun point d'instrumentation n'existe encore dans le pipeline (rien à mesurer avant B/C), le câblage est un suivi à faire quand il y aura une latence réelle à observer — pas un travail perdu, juste prématuré de forcer un appel sans rien à instrumenter.

## B — Tracking-by-detection

Référence matrice : B — **boucle de ré-ancrage vivante depuis le 2026-08-09**, branchée dans `main.go`/`UseCase`. Seul le test de dérive KCF vs CSRT reste ouvert (nécessite du matériel vidéo réel, pas juste du code).

- [x] Réutiliser l'IoU existant plutôt que le réécrire : fait le 2026-08-05, en même temps que la sortie de `go-clean-onnxruntime` (voir `github.com/deadelus/go-clean-onnxruntime` vs `onnxruntime_go`, décision prise en aparté) — `entities.BoundingBox.IoU/Intersection/Union` dans `internal/domain/entities/boundingbox.go`, utilisé pour le NMS de `yolo11s.go`. Publique, testable, plus de dépendance au vendor pour ça.
- [x] Adapter KCF/CSRT derrière `infrastructure/tracking.ObjectTracker` — `internal/implementation/tracking/gocv-tracker` (`Tracker`, algo sélectionnable via `New(KCF|CSRT)`, un seul adapter pour les deux : le choix reste ouvert tant que le test de dérive n'est pas fait). Vendoring `gocv.io/x/gocv/contrib` ajouté (`go mod vendor`). Tests unitaires (`tracker_test.go`, table-driven) : construction, erreurs, `Cleanup()` idempotent avant `Init`. **MIL non adapté** (réserve si KCF/CSRT déçoivent, cf. `docs/adr/object-tracking.md` § 5) — pas branché dans `main.go`/`UseCase` : rien ne consomme encore `ObjectTracker` dans le pipeline (voir item suivant).
- [x] Boucle de ré-ancrage périodique + association IoU contre les tracks existantes — `internal/application/uc/tracking.go` (`trackManager`). Re-détection YOLO toutes les `reanchorInterval=45` frames (~1.5s à 30fps, **pas mesuré**, cf. F) plutôt que "sur chute de confiance" (aurait demandé d'exposer la confiance du tracker lui-même, que gocv ne fournit pas — KCF/CSRT ne renvoient qu'un booléen succès/échec, pas de score). Association gloutonne par IoU (seuil `iouAssociationThreshold=0.4`, milieu de la fourchette 0.3-0.5), restreinte à la même classe, pas d'algorithme hongrois (suffisant pour une v1). `RecognitionUseCase` réécrit pour consommer `trackManager` au lieu de redétecter à chaque frame — corrige au passage un bug latent (les events clavier n'étaient jamais lus si `BoundingBoxes` était `nil`, `Escape` pouvait rester sans effet) et réactive le matching de filtre + `notifier.Notify`, qui était mort en commentaire depuis le début.
- [x] Test de dérive KCF vs CSRT sur vidéo réelle — `cmd/tracking-drift` (outil headless jetable), résultats et décision dans `docs/adr/object-tracking.md` § 7-8. **KCF confirmé comme défaut** (avg IoU supérieur à CSRT sur les deux vidéos testées, `person.mp4` + `car.mp4`), déjà le choix câblé dans `main.go`. Échantillon petit (2 vidéos) — pas définitif, CSRT reste à un changement d'une ligne si besoin.

## D — Agrégat `Track`

Référence matrice : D — **type domaine + événements faits le 2026-08-09.** Le 3e item (scission `SemanticFilter`) reste hors scope tant que A n'a pas de CLIP qui tourne, volontairement non commencé.

- [x] Créer le type domaine `Track` (`internal/domain/entities/track.go`) : `ID`, `Class`, `Trajectory []BoundingBox`, `Embeddings []Embedding` (vide tant que A n'existe pas), `FirstSeen`/`LastSeen`, machine à états `StateTentative → StateConfirmed → StateCoasting → StateLost` via `MatchDetection`/`Coast`/`Miss`. Seuils `minHitsToConfirm=3`/`maxMissesBeforeLost=5` posés en dur (valeurs SORT-like par défaut, **non mesurées** — à revoir une fois la boucle de ré-ancrage tournant sur vidéo réelle, cf. F). 11 tests table-driven (`track_test.go`), 100% des transitions couvertes.
- [x] Émettre les événements `TrackEntered`/`TrackMatched`/`TrackLost` — mécanisme créé (`MatchDetection`/`Miss` retournent un `*TrackEvent`, pur/sans I/O côté domaine) **et câblé** : `trackManager.emit` (`internal/application/uc/tracking.go`) route vers `AlertSender.Notify` quand la classe du track correspond au filtre demandé (`req.Filter`) au-dessus du seuil (`req.SimilarityThreshold`). `EventTrackLost` est loggé mais n'alerte jamais (rien à notifier sur une disparition).
- [ ] Scinder `SemanticFilter` en filtres **instantanés** (crop) et filtres **temporels** (règles sur trajectoire). **Dépendance : Track + décision A (CLIP) pour les filtres instantanés. Effort : M, et dépend fortement de la forme finale de A — ne pas figer le design avant d'avoir un CLIP qui tourne.**

## C — Découplage async 3 boucles

Référence matrice : C — DIVERGENT. Un prototype existe sur `feat/ochestrator` mais à 1 seule boucle, code jetable (le `main.go` de cette branche panique volontairement en fin d'exécution). **Ne pas merger cette branche telle quelle — voir question ouverte dans AUDIT.md.**

- [ ] Réécrire la boucle vidéo pour qu'elle ne bloque jamais (lecture des résultats de détection en `select`/`default`). **Dépendance : tracker (B) pour que la boucle vidéo ait quelque chose à faire à 25-30 FPS pendant que la détection tourne plus lentement. Effort : M.**
- [ ] Channel de frames à détecter en buffer 1 avec écrasement — **le prototype `feat/ochestrator` a déjà cette logique dans `orchestrator.go` (lignes ~500-513), réutilisable presque telle quelle pour ce sous-problème précis** (mais pas pour l'orchestration complète). **Dépendance : aucune. Effort : XS, code de référence déjà écrit.**
- [ ] Boucle de détection séparée à 2-5 FPS sur sa propre goroutine. **Dépendance : channel ci-dessus. Effort : S.**
- [ ] Boucle sémantique (CLIP) déclenchée à l'événement (1× par track confirmée), pas par frame. **Dépendance : Track (D) + CLIP (A). Effort : S une fois les deux prêts.**
- [ ] V1 recalage naïf (le tracker rattrape le décalage) — à livrer avant V2. **Dépendance : boucles ci-dessus. Effort : XS.**
- [ ] V2 ring buffer des ~15 dernières frames + rejeu tracker — amélioration ultérieure, pas bloquante pour un premier livrable. **Dépendance : V1. Effort : M, à ne faire qu'après retour d'usage sur V1.**

## A — Cascade YOLO → crop → CLIP

Référence matrice : A — cascade et CLIP ABSENTS. Segmentation **[DÉJÀ FAIT → À INTÉGRER]** dans `stash@{0}`.

- [x] Récupérer `stash@{0}` — **fait le 2026-08-06, mais pas comme prévu** : le stash lui-même avait déjà disparu (`git stash list` vide, reflog absent) au moment de reprendre ce chantier, seul le commit orphelin (`fb02e91`, objet non garbage-collecté) subsistait. Récupéré sur `recover/yoloe11s-seg-stash`. Contient : `yoloe11s-seg` complet (détection + segmentation), `AI.AnalyzeFrame(frame, filters)` avec filtrage par classe, `Render()` renvoyant `(bool, error)`. **Reste à faire** : merger/porter ce contenu dans la structure actuelle (`internal/implementation/inference/...`), le stash date d'avant la migration Phase G donc son arborescence (`src/...`) est obsolète.
- [ ] Choisir et intégrer un backend CLIP en ONNX (ViT-B/32 ou équivalent léger) suivant le même schéma que `yolo11s.go`/`yoloe11s-seg.go` (déjà éprouvé deux fois). **Dépendance : aucune technique, mais logiquement après la récupération du stash pour éviter le travail en double sur la structure `implementation/ai/`. Effort : L (2-3 j, includes export du modèle + intégration Go).**
- [ ] Crop de la frame sur la bbox YOLO avant envoi à CLIP (pas la frame entière). **Dépendance : tracker (B) pour avoir une bbox stable à chaque frame, sinon crop sur la bbox de détection brute en attendant. Effort : S.**
- [ ] Calcul des embeddings texte des filtres une seule fois au démarrage (pas par frame). **Dépendance : CLIP intégré ci-dessus. Effort : XS.**
- [ ] Garder la segmentation (masque pixel, récupérée du stash) en option explicite, pas en comportement par défaut — seulement utile sur scènes très encombrées. **Dépendance : intégration du stash. Effort : XS, il s'agit surtout de ne pas l'activer par défaut.**

## Bugs UX ponctuels (indépendants du séquencement G→F)

- [ ] Effet miroir sur le flux webcam : l'image rendue est inversée gauche/droite par rapport à l'utilisateur (comportement webcam brut, non-mirroré). Flip horizontal à appliquer sur le `gocv.Mat` lu (`gocv.Flip(imgMat, &imgMat, 1)`) dans `internal/implementation/streamer/input/camera.go` (`CameraInput.Start`, après `ci.camera.Read(&imgMat)` ligne ~45), avant la conversion `ToImage()`. **Dépendance : aucune. Effort : XS.**

## F — Optimisations transverses (en continu, pas une phase isolée)

Référence matrice : F — PARTIEL, l'ordre de préférence ONNX-first est trivialement respecté (seul backend existant), le reste dépend de A.

- [ ] Benchmarker réellement la latence CLIP une fois intégré (décision A) sur CPU x86 et si possible ARM, pour remplacer les chiffres de la mission (indicatifs) par des mesures réelles du projet. **Dépendance : A. Effort : S.**
- [ ] Documenter l'ordre de fallback ONNX Go natif > Python embarqué > REST local > API cloud dans le code (commentaire sur l'interface `ObjectDetector`/`SemanticEncoder`), même si seul ONNX est implémenté pour l'instant — pour que le prochain contributeur comprenne l'intention sans avoir à lire le README aspirant. **Dépendance : E. Effort : XS.**
- [x] Corrigé le 2026-08-09 (partiel — voir item suivant, pas la cause principale) : `gocvtracker.Tracker.Init`/`Update` reconvertissait `image.Image → gocv.Mat` indépendamment pour chaque track actif au lieu de partager la conversion. Fix : cache par identité de pointeur `*entities.Frame` (`sharedFrameMat`). Conversion mesurée à 0.3-1.6ms en réalité (pas ~100ms comme estimé initialement) — gain réel mais pas la source du ralentissement observé.
- [x] **Cause racine trouvée le 2026-08-09 via `sample` (profiler CLI macOS, pas de Xcode/Instruments nécessaire)** : `KCF/CSRT backend.Update()` passait ~330 échantillons sur 10s dans `TrackerKCFImpl::updateProjectionMatrix → oclTransposeMM → cv::ocl::Kernel::run → clFinish (OpenCL) → AppleIntelKBLGraphicsGLDriver → IOAccelContextFinish → mach_msg2_trap`. KCF utilise le T-API d'OpenCV (`UMat`/OpenCL) pour décharger une transposition de matrice sur le GPU intégré Intel — la synchro GPU (`clFinish`) coûte des centaines de ms sur ce driver, largement plus cher que le calcul CPU direct qu'elle est censée accélérer (gotcha connu d'OpenCV T-API sur petites matrices/iGPU faible). Rien à voir avec les threads ORT, `SetNumThreads`, ou `VECLIB_MAXIMUM_THREADS` (les deux testés et infirmés avant celui-ci). **Suivi** : désactiver le T-API/OpenCL pour ce module (`OPENCV_OPENCL_DEVICE=disabled` à tester, ou binding gocv si un existe pour `cv::ocl::setUseOpenCL(false)` — pas encore vérifié côté correctif permanent dans le code, testé seulement en variable d'env manuelle).
- [x] Bug distinct trouvé pendant l'investigation (2026-08-09) : le filtre de classe (`req.Filter`) n'était appliqué qu'à l'alerte, jamais au tracking — tout objet détecté était suivi/dessiné (pré-existant, jamais un vrai filtre avant le rewrite § B). Corrigé (`trackManager.reanchor`). **Deuxième bug trouvé immédiatement après, à cause du premier fix** : le filtre n'était jamais trim/lowercase — une saisie CLI avec un espace parasite (`" person"` au lieu de `"person"`) filtrait silencieusement *toutes* les détections, sans erreur, symptôme identique à "la détection est cassée". `normalizeFilter` (trim + lowercase) ajouté à chaque point de comparaison (`reanchor`, `emit`) plutôt que de faire confiance à un seul point d'entrée.

---

## Hors-scope de ce backlog

- **Décision H (topologie de déploiement)** : question ouverte, non transformée en tâche. Voir `AUDIT.md` § Décision H et `MIGRATION.md` § Risques.
- **Branches à trancher** (`realtime-uc`, fast-forward `main`) : décisions utilisateur, voir `AUDIT.md` § Questions. `feat/ochestrator` est tranchée : non récupérée (le commit dangling `4c482dc` a été supprimé volontairement, pas de branche de secours).
