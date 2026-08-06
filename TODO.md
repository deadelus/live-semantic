# TODO — LiveSemantic

Backlog dérivé des décisions d'architecture validées (voir contexte de mission). Chaque tâche référence sa ligne dans `AUDIT.md` § Étape 2bis. Les tâches marquées **[DÉJÀ FAIT → À INTÉGRER]** ne sont pas à recoder : le travail existe mais est non commité (stash) ou non mergé (branche) — le premier geste est de le récupérer, pas de le réécrire.

Ordre de dépendance global : **G (hygiène) → E (ports) → B (IoU/tracker) → D (Track) → C (async) → A (cascade) → F (latences, en continu)**. H reste une question ouverte, pas une tâche.

---

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

Référence matrice : B — tracker ABSENT. La primitive IoU (ex-vendorisée dans `go-clean-onnxruntime`) est maintenant domaine.

- [x] Réutiliser l'IoU existant plutôt que le réécrire : fait le 2026-08-05, en même temps que la sortie de `go-clean-onnxruntime` (voir `github.com/deadelus/go-clean-onnxruntime` vs `onnxruntime_go`, décision prise en aparté) — `entities.BoundingBox.IoU/Intersection/Union` dans `internal/domain/entities/boundingbox.go`, utilisé pour le NMS de `yolo11s.go`. Publique, testable, plus de dépendance au vendor pour ça.
- [ ] Intégrer un tracker mono-objet gocv (KCF, CSRT ou MOSSE au choix — à trancher selon un test de dérive sur vidéo réelle, aucun des trois n'est présupposé meilleur ici). **Dépendance : `ports.ObjectTracker` (E). Effort : M (0.5-1 j, gocv expose déjà ces trackers nativement, donc pas de portage à faire, juste l'intégration + tests de dérive).**
- [ ] Boucle de ré-ancrage périodique (re-détection toutes les 1-2s ou sur chute de confiance) + association IoU (seuil 0.3-0.5) contre les tracks existantes. **Dépendance : tracker ci-dessus + agrégat Track (D). Effort : M.**

## D — Agrégat `Track`

Référence matrice : D — ABSENT partout, dépend de B.

- [ ] Créer le type domaine `Track` (id, classe, trajectoire, embeddings agrégés, first/last seen) et sa machine à états `Tentative → Confirmed → Coasting → Lost`. **Dépendance : tracker fonctionnel (B), au moins en version minimale. Effort : M (0.5-1 j pour le type + tests unitaires de transition d'état).**
- [ ] Émettre les événements `TrackEntered` / `TrackMatched` / `TrackLost` au lieu d'une alerte par frame. **Dépendance : Track ci-dessus + `AlertSender` (E). Effort : S.**
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

---

## Hors-scope de ce backlog

- **Décision H (topologie de déploiement)** : question ouverte, non transformée en tâche. Voir `AUDIT.md` § Décision H et `MIGRATION.md` § Risques.
- **Branches à trancher** (`realtime-uc`, fast-forward `main`) : décisions utilisateur, voir `AUDIT.md` § Questions. `feat/ochestrator` est tranchée : non récupérée (le commit dangling `4c482dc` a été supprimé volontairement, pas de branche de secours).
