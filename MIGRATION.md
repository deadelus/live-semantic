# MIGRATION — LiveSemantic

Plan de mise en œuvre, construit à partir de l'union de tout ce qui existe réellement (branches + stashes recensés dans `AUDIT.md`), pas seulement de `main`. **Aucune phase ci-dessous n'a été exécutée** — ce document est une proposition à valider avant toute action sur le code.

---

## Phase 0 — Consolidation de l'existant dispersé

Objectif : rassembler le travail avant de construire quoi que ce soit de neuf. Rien de nouveau n'est écrit dans cette phase.

**Actions, avec le pourquoi de chaque mouvement :**

1. `git stash branch salvage/yoloe11s-seg stash@{0}` → nouvelle branche dédiée, puis commit. *Pourquoi maintenant* : c'est le seul artefact avec un risque réel de perte définitive (un `git gc` ou un `stash drop` accidentel l'effacerait) ; tout le reste est déjà dans un commit quelque part.
2. Vérifier `stash@{1}` est bien superseded par `stash@{0}` (diff déjà fait dans `AUDIT.md` : oui pour les fichiers communs) puis `git stash drop stash@{1}` — après confirmation utilisateur (question posée dans `AUDIT.md`).
3. `feat/ochestrator` : ne pas merger. Extraire uniquement la logique du channel à buffer-1-avec-écrasement (`orchestrator.go` L500-513) sous forme d'un gist/snippet de référence conservé dans `TODO.md` § C (déjà fait dans ce document), puis geler ou supprimer la branche selon la réponse utilisateur.
4. `realtime-uc` : confirmée entièrement absorbée par `feat/displayer` (ancêtre direct) → suppression sans perte, après confirmation utilisateur.
5. `main` : fast-forward depuis `feat/displayer` une fois la Phase 1 (hygiène) terminée sur `feat/displayer`, pas avant — pour ne jamais publier sur `main` un état contenant `.env` en clair ou les placeholders README. *Pourquoi attendre* : `main` est la branche par défaut, donc la plus visible publiquement ; autant n'y faire atterrir qu'un état déjà nettoyé plutôt que de nettoyer après-coup sur la branche par défaut.

**Vérification** : `git log --all --oneline` ne doit plus faire apparaître de commit accessible uniquement via une branche/stash orpheline (hormis celles explicitement gelées avec l'accord utilisateur).

**Risque** : aucune perte de travail attendue si l'ordre ci-dessus est respecté (le salvage du stash passe avant tout nettoyage). Le seul point irréversible est la suppression de branches — à ne faire qu'après confirmation explicite (voir `AUDIT.md` § Questions).

---

## Phase 1 — Hygiène et restructuration (`src/` → `cmd/` + `internal/`)

Risque faible, débloque tout le reste (Go modules pruning, imports internes, lisibilité). Correspond à la décision G.

**Fichiers déplacés :**

```
src/main.go                    → cmd/livesemantic/main.go
src/domain/                    → internal/domain/        (Track, Match, Filter à créer ici en Phase 2)
src/infrastructure/            → internal/ports/          (renommage sémantique : ce sont des ports, pas de l'infra)
src/implementation/            → internal/infrastructure/ (renommage sémantique inverse : ce sont les adapters)
src/internal/drawer/           → internal/infrastructure/drawer/
src/transport/                 → internal/transport/      (déplace, ne réécrit pas)
```

*Pourquoi ce remapping de noms* : la convention Clean Architecture standard veut que les *interfaces* (ports) soient nommées `ports/` et les *implémentations concrètes* `infrastructure/`. Le projet actuel a l'inverse (`infrastructure/` = interfaces, `implementation/` = adapters), ce qui a probablement dérouté la lecture lors de la rédaction de la mission initiale (les décisions ont été prises sans lire le code). On aligne les noms sur la convention plutôt que d'inventer une troisième nomenclature.

**Fichiers créés :**
- `.env.example` (copie de `.env` sans les valeurs), puis `git rm --cached .env`.
- `LICENSE` (MIT).

**Fichiers modifiés :**
- `readme.md` : placeholders `your-org` → `deadelus`, chiffres de latence corrigés.
- Tous les fichiers Go déplacés : chemins d'import à réécrire (`live-semantic/src/...` → `live-semantic/internal/...` ou `live-semantic/cmd/...`).

**Ce qui casse et comment vérifier :**
- Compilation cassée le temps du déplacement (imports). Vérification : `go build ./...` doit repasser au vert avant de committer la phase.
- `go vet ./...` et `go test ./...` doivent rester verts (les 2 tests existants ne doivent pas régresser).
- Test de non-régression manuel : `go run ./cmd/livesemantic recognition --filter=person` doit démarrer la capture webcam et afficher la fenêtre, comme aujourd'hui avec `go run ./src`.
- Le binaire de vendoring (`vendor/modules.txt`) n'a pas besoin de changer (aucune dépendance externe ne bouge dans cette phase) — seul `go mod vendor` de contrôle pour confirmer.

**Livrable de la phase** : un commit unique de déplacement (pas de logique changée), un commit séparé pour l'hygiène `.env`/`LICENSE`/`README`. Deux commits distincts pour que le déplacement pur reste facile à `git blame`/revert indépendamment du nettoyage.

---

## Phase 2 — Domaine et ports

Correspond aux décisions D (Track) et E (ports). Dépend de la Phase 1 (les nouveaux fichiers atterrissent directement dans `internal/domain` et `internal/ports`).

**Fichiers créés :**
- `internal/domain/track.go` — type `Track` + machine à états (Tentative/Confirmed/Coasting/Lost).
- `internal/domain/events.go` — `TrackEntered`, `TrackMatched`, `TrackLost`.
- `internal/ports/object_detector.go`, `object_tracker.go`, `semantic_encoder.go`, `metrics_collector.go` — nouveaux ports.

**Fichiers modifiés :**
- `internal/ports/ai.go` (ex-`infrastructure/ai/neural_model.go`) : `AI` renommé/scindé en `ObjectDetector`, `DetectionResult.BoundingBoxes` reprend un type domaine propre au lieu de `onnx.BoundingBox`.
- `internal/ports/notifier.go` (ex-`notifier.Notifier`) : conservé si le contrat correspond déjà à `AlertSender`, sinon renommé.

**Ce qui casse et comment vérifier :**
- Tout appelant de `ai.AI.AnalyzeFrame` (aujourd'hui : `uc_recognition.go`, `yolo11s.go`) doit être mis à jour pour le nouveau contrat `ObjectDetector`. `go build ./...` détecte immédiatement les sites à corriger (le compilateur Go est le filet de sécurité ici).
- `domain/uc/init_test.go` (actuellement sans test réel exécuté) doit gagner au moins un test de transition d'état pour `Track`, sinon la Phase 2 introduit un agrégat central sans aucune garantie de correction — à ne pas livrer sans ce test minimal.

**Ce qui est réutilisé tel quel** : le câblage dans `main.go` (constructeurs → injection dans `uc.NewUseCase`) — le principe DI actuel est déjà correct (confirmé en audit), seule la forme des types injectés change.

---

## Phase 3 — Infrastructure vidéo (tracking + async)

Correspond aux décisions B et C. Dépend de la Phase 2 (le port `ObjectTracker` doit exister).

**Fichiers créés :**
- `internal/infrastructure/tracker/gocv_tracker.go` — wrapper autour de `gocv.NewTrackerKCF`/`CSRT`/`MOSSE` (à trancher par un test de dérive rapide, pas par supposition).
- `internal/infrastructure/pipeline/` — les 3 boucles découplées (vidéo/détection/sémantique), en s'inspirant du channel buffer-1-écrasement déjà écrit dans `feat/ochestrator:src/orchestrator/orchestrator.go` (référence, pas un merge direct — voir `AUDIT.md` décision C).

**Fichiers modifiés :**
- `internal/domain/uc/uc_recognition.go` : le callback `frameActionCallback` synchrone actuel (`Start(func(frame) (*Frame, error))`) doit devenir compatible avec un appel non-bloquant — c'est le changement le plus structurant de cette phase, à isoler dans son propre commit pour rester bisectable.

**Ce qui casse et comment vérifier :**
- Le mode CLI `recognition` actuel est strictement synchrone (une frame in, une frame out, séquentiellement). Le passage à 3 boucles change fondamentalement la latence perçue et l'ordre d'affichage — à valider visuellement (la fenêtre ne doit pas saccader plus qu'avant) en plus de `go build`/`go test`.
- Ajouter un test de charge simple (mesurer FPS affiché avant/après) pour objectiver le gain annoncé par la décision C plutôt que de le supposer.

**Risque à arbitrer** : le décalage temporel V1 (le tracker rattrape le recalage tel quel) peut produire des bbox visiblement en retard sur l'affichage à la première itération — acceptable en V1 selon la mission, mais à confirmer visuellement avant de considérer la phase terminée.

---

## Phase 4 — IA cascade (CLIP)

Correspond aux décisions A et F. Dépend de la Phase 3 (le crop utilise la bbox du tracker, pas seulement celle de la détection brute).

**Fichiers créés :**
- `internal/infrastructure/ai/clip/clip.go` — nouveau backend, même schéma que `yolo11s.go`/`yoloe11s-seg.go` (déjà éprouvé deux fois, donc faible risque d'intégration ONNX).
- Récupération de `internal/infrastructure/ai/yoloe11sseg/` depuis `salvage/yoloe11s-seg` (Phase 0), déplacé dans la nouvelle arborescence.

**Fichiers modifiés :**
- `internal/domain/uc/uc_recognition.go` : ajout du crop bbox → appel CLIP → comparaison cosine, en aval du tracker.
- `internal/domain/filter.go` (nouveau ou existant selon Phase 2) : scission filtres instantanés / temporels.

**Ce qui casse et comment vérifier :**
- Le contrat `AnalyzeFrame(frame, filters)` de `stash@{0}` (déjà migré en Phase 0) prend une liste de classes fermée YOLO — la décision A veut un vocabulaire ouvert CLIP en complément, pas en remplacement. Vérifier explicitement dans les tests que YOLO reste utilisé pour la détection et CLIP uniquement pour le crop, pour ne pas régresser vers un appel CLIP par frame entière (c'est exactement l'anti-pattern que la décision A veut éviter).
- Benchmark de latence réel (tâche F du `TODO.md`) à exécuter en fin de phase, pas avant — les chiffres de la mission sont indicatifs, pas une spec à respecter au chiffre près.

---

## Réutilisé tel quel (confirmé par lecture du code, pas supposé)

- **Transport CLI (cobra + interactif)** : solide, branché de bout en bout sur `RecognitionUseCase`, testé en conditions réelles (binaire exécuté avec succès pendant la migration `go-clean-app` v2 du 2026-08-04). Aucune réécriture nécessaire au-delà du déplacement de fichiers en Phase 1.
- **Pipeline YOLO11s → ONNX → gocv** : fonctionne, sert de modèle pour l'intégration CLIP (Phase 4) et a déjà servi deux fois (YOLO11s, YOLOE11s-seg dans le stash) — le schéma est éprouvé, pas à réinventer.
- **DI dans `main.go`** : le principe (constructeurs concrets injectés dans les use cases via interfaces) est correct, seule la forme des interfaces change en Phase 2.
- **Shutdown gracieux + logger zap (`go-clean-app` v2)** : fraîchement migré et testé, rien à toucher.

**Non réutilisable tel quel, à documenter comme tel pour ne pas le recharger par erreur** : le transport Web API/WebSocket (squelettes vides, voir `AUDIT.md`) et le prototype `feat/ochestrator` (voir décision C).

---

## Risques et arbitrages nécessaires

1. **Suppression de branches/stash** (Phase 0) : irréversible, nécessite l'accord explicite de l'utilisateur avant exécution — questions posées dans `AUDIT.md`.
2. **Réécriture d'historique pour les binaires committés** (`.onnx`, `.so`/`.dylib`/`.dll`, 45 Mo+ de `vendor/` et bien plus dans `.git`) : si on décide de les retirer de l'historique (pas juste du futur), c'est un `git filter-repo` ou équivalent — opération destructive sur un repo public partagé, à ne faire qu'avec un accord explicite et une communication (les URLs de commit changeraient). **Proposition par défaut, moins risquée : ne pas réécrire l'historique existant, seulement arrêter d'y ajouter de nouveaux binaires (Git LFS ou download-at-build à partir de maintenant).**
3. **Décision H (topologie de déploiement)** : bloquante pour la forme finale du port `VideoSource` vs `FrameIngress` (pull vs push). Aucune phase ci-dessus n'en dépend directement (tout le plan suppose une machine unique, cohérent avec le code actuel), mais si la réponse est "flotte edge", la Phase 3 devra être revue avant d'être considérée terminée. **Ne pas commencer la Phase 3 sans réponse à cette question**, pour éviter de construire un port `VideoSource` en pull qu'il faudra jeter.
4. **Choix du tracker (KCF/CSRT/MOSSE)** : la mission ne tranche pas lequel, seulement la famille. Proposition : un test de dérive de 30 secondes sur une vidéo réelle avec les trois, décision sur mesure plutôt que sur réputation — à faire en tout début de Phase 3, pas avant (pas de valeur à trancher ça avant d'avoir le port `ObjectTracker` en place).
5. **`feat/ochestrator` non mergée** : si l'utilisateur préfère la merger malgré tout (par exemple pour préserver l'historique Git des idées), le `main.go` cassé (panique volontaire) devra être nettoyé avant tout merge — ne jamais merger une branche dont le point d'entrée panique intentionnellement sans corriger ce point en premier.
