# TODO — LiveSemantic

Backlog dérivé des décisions d'architecture validées (voir contexte de mission). Chaque tâche référence sa ligne dans `AUDIT.md` § Étape 2bis. Les tâches marquées **[DÉJÀ FAIT → À INTÉGRER]** ne sont pas à recoder : le travail existe mais est non commité (stash) ou non mergé (branche) — le premier geste est de le récupérer, pas de le réécrire.

Ordre de dépendance global : **G (hygiène) → E (ports) → B (IoU/tracker) → D (Track) → C (async) → A (cascade) → F (latences, en continu)**. H reste une question ouverte, pas une tâche.

---

## G — Restructuration & hygiène (bloquant, faible risque, à faire en premier)

Référence matrice : G — restructuration absente partout, problèmes d'hygiène confirmés en étape 2.

- [ ] `git rm --cached .env`, créer `.env.example` avec les 4 clés (`APP_NAME`, `APP_VERSION`, `APP_ENV`, `APP_DEBUG`) sans valeurs sensibles. **Dépendance : aucune. Effort : XS (15 min).**
- [ ] Ajouter `LICENSE` (MIT, comme annoncé dans le badge du README). **Dépendance : aucune. Effort : XS (5 min).**
- [ ] Remplacer les placeholders `github.com/your-org/livesemantic` par `github.com/deadelus/live-semantic` dans `readme.md` (lignes 5 et 48). **Dépendance : aucune. Effort : XS (5 min).**
- [ ] Corriger les promesses de latence non étayées ("sub-50ms") dans `readme.md` avec les chiffres réalistes de la décision F (20-40 ms CPU x86, 300ms-1s ARM fp32). **Dépendance : aucune. Effort : XS.**
- [ ] Sortir les binaires (`*.onnx`, `onnxruntime.{so,dylib,dll}`) de l'historique Git courant vers Git LFS ou un téléchargement au build. **Dépendance : décision utilisateur sur la méthode (LFS vs script de download) — à arbitrer, voir `MIGRATION.md`. Effort : M (0.5-1 j), plus risque sur l'historique si réécriture.**
- [ ] Migration `src/` → `cmd/livesemantic/` + `internal/` selon la cible de `MIGRATION.md`. **Dépendance : aucune techniquement, mais à faire une fois les stashes/branches consolidés (voir AUDIT.md § questions branches) pour éviter de dupliquer le travail de déplacement. Effort : M (0.5 j, majoritairement mécanique).**
- [ ] Déplacer les DTO actuels (`src/domain/dto`) vers la couche applicative une fois la restructuration faite. **Dépendance : migration `src/`→`internal/` ci-dessus. Effort : XS.**

## E — Ports (inversion de dépendance complète)

Référence matrice : E — PARTIEL/DIVERGENT. La DI existe déjà (`infrastructure/ai.AI`, `streamer.{Input,Output}Stream`, `notifier.Notifier`), mais la forme ne correspond pas à la cible.

- [ ] Scinder `infrastructure/ai.AI` (actuel, mélange détection) en `ports.ObjectDetector` (retourne `[]DetectedObject{Class, BBox, Score}`) — renommage/reshape de l'existant, pas une création ex nihilo. **Dépendance : restructuration G (pour atterrir dans `internal/ports/`). Effort : S (0.5 j).**
- [ ] Corriger la fuite d'abstraction : `DetectionResult.BoundingBoxes` ne doit plus être typé `[]onnx.BoundingBox` (vendor) mais un type domaine propre. **Dépendance : aucune, indépendant du reste. Effort : XS-S.**
- [ ] Créer `ports.ObjectTracker` (`Init(frame, box)` / `Update(frame)`) — nouveau, rien à récupérer. **Dépendance : choix du tracker gocv, voir B. Effort : XS (juste l'interface).**
- [ ] Créer `ports.SemanticEncoder` (retourne `Embedding`) — nouveau. **Dépendance : aucune pour l'interface seule ; l'implémentation dépend de A. Effort : XS.**
- [ ] `ports.AlertSender` : renommer/formaliser l'actuel `notifier.Notifier` si le contrat correspond déjà, sinon l'adapter. **Dépendance : aucune. Effort : XS.**
- [ ] Créer `ports.MetricsCollector` — nouveau, rien n'existe. **Dépendance : aucune pour l'interface ; implémentation console d'abord (cohérent avec Phase 1 du roadmap `overview.md`). Effort : S.**

## B — Tracking-by-detection

Référence matrice : B — tracker ABSENT, mais primitive IoU déjà écrite et testée (NMS) dans `vendor/github.com/deadelus/go-clean-onnxruntime/src/onnx/boundingbox.go`.

- [ ] **[DÉJÀ FAIT → À INTÉGRER]** Réutiliser l'IoU existant plutôt que le réécrire : soit l'exposer publiquement côté lib vendorisée (PR sur `go-clean-onnxruntime`), soit dupliquer les ~10 lignes côté domaine `live-semantic` (plus rapide, moins de couplage). **Dépendance : ports E (le type domaine bbox doit exister d'abord). Effort : XS une fois E fait.**
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

- [ ] **Récupérer `stash@{0}` sur une branche dédiée avant toute autre chose sur ce chantier** (risque de perte sinon). Contient : `yoloe11s-seg` complet (détection + segmentation), `AI.AnalyzeFrame(frame, filters)` avec filtrage par classe, `Render()` renvoyant `(bool, error)`. **Dépendance : aucune, c'est un `git stash branch` ou `pop`. Effort : XS (15 min) — mais à faire en tout premier, avant de toucher à quoi que ce soit d'autre sur cette branche de travail.**
- [ ] Choisir et intégrer un backend CLIP en ONNX (ViT-B/32 ou équivalent léger) suivant le même schéma que `yolo11s.go`/`yoloe11s-seg.go` (déjà éprouvé deux fois). **Dépendance : aucune technique, mais logiquement après la récupération du stash pour éviter le travail en double sur la structure `implementation/ai/`. Effort : L (2-3 j, includes export du modèle + intégration Go).**
- [ ] Crop de la frame sur la bbox YOLO avant envoi à CLIP (pas la frame entière). **Dépendance : tracker (B) pour avoir une bbox stable à chaque frame, sinon crop sur la bbox de détection brute en attendant. Effort : S.**
- [ ] Calcul des embeddings texte des filtres une seule fois au démarrage (pas par frame). **Dépendance : CLIP intégré ci-dessus. Effort : XS.**
- [ ] Garder la segmentation (masque pixel, récupérée du stash) en option explicite, pas en comportement par défaut — seulement utile sur scènes très encombrées. **Dépendance : intégration du stash. Effort : XS, il s'agit surtout de ne pas l'activer par défaut.**

## F — Optimisations transverses (en continu, pas une phase isolée)

Référence matrice : F — PARTIEL, l'ordre de préférence ONNX-first est trivialement respecté (seul backend existant), le reste dépend de A.

- [ ] Benchmarker réellement la latence CLIP une fois intégré (décision A) sur CPU x86 et si possible ARM, pour remplacer les chiffres de la mission (indicatifs) par des mesures réelles du projet. **Dépendance : A. Effort : S.**
- [ ] Documenter l'ordre de fallback ONNX Go natif > Python embarqué > REST local > API cloud dans le code (commentaire sur l'interface `ObjectDetector`/`SemanticEncoder`), même si seul ONNX est implémenté pour l'instant — pour que le prochain contributeur comprenne l'intention sans avoir à lire le README aspirant. **Dépendance : E. Effort : XS.**

---

## Hors-scope de ce backlog

- **Décision H (topologie de déploiement)** : question ouverte, non transformée en tâche. Voir `AUDIT.md` § Décision H et `MIGRATION.md` § Risques.
- **Branches à trancher** (`feat/ochestrator`, `realtime-uc`, fast-forward `main`) : décisions utilisateur, voir `AUDIT.md` § Questions.
