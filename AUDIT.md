# AUDIT — LiveSemantic

Date : 2026-08-04. Repo : `deadelus/live-semantic` (GitHub, **public**, confirmé via API). Mission en lecture seule — aucun fichier de code n'a été modifié pour produire cet audit (seuls `overview.md`, `di-architecture-example.md` — renommage de l'ancien `todo.md` — et les trois livrables `AUDIT.md`/`TODO.md`/`MIGRATION.md` ont été touchés, à la demande explicite préalable de l'utilisateur, hors périmètre de cette mission).

> ## ⚠️ Mise à jour de recadrage — 2026-08-13
>
> **Tout ce qui suit ce bloc est un instantané figé au 2026-08-04, conservé
> tel quel comme trace historique — il ne décrit plus l'état du repo.**
> Ne pas s'y fier pour juger de l'avancement actuel : voir
> `docs/gui/spec.md` (mis à jour le 2026-08-13) et le code lui-même
> (`internal/`, `cmd/`, `web/`) pour l'état réel.
>
> Ce qui a changé depuis, en bref :
> - **Restructuration `src/` → `cmd/`+`internal/` faite** (décision G) —
>   la structure décrite ci-dessous (`src/domain`, `src/infrastructure`,
>   etc.) n'existe plus, voir `internal/{domain,application,infrastructure,
>   implementation,transport}` + `cmd/livesemantic`.
> - **`main` n'est plus en retard** : les branches de feature citées ici
>   (`realtime-uc`, `feat/ochestrator`, `feat/displayer`) ont soit été
>   mergées soit abandonnées ; tout le travail se fait maintenant par
>   PR mergées sur `main` (voir `git log`).
> - **API web (gin) et WebSocket (gorilla) sont câblées** sur la logique
>   métier depuis le 2026-08-10 — le constat "zéro appel à
>   `RecognitionUseCase`" ci-dessous (Étape 2, "Les transports
>   partagent-ils vraiment la même logique métier ?") est **obsolète**.
>   S'y ajoute depuis le 2026-08-12 un vrai multi-flux (`session.Manager`)
>   et une galerie de références CRUD (`/api/v1/gallery`).
> - Un frontend web (`web/`, React + Vite + TypeScript) existe, basculé le
>   2026-08-13 sur l'API multi-flux (`/api/v1/sessions/*`,
>   `/ws/sessions/:id`) — un seul flux affiché pour l'instant (pas encore
>   de vue liste/mosaïque multi-source), et sans UI pour la galerie.
> - Le prototype de segmentation `stash@{0}` évoqué ci-dessous (§ Étape 1,
>   décision A) n'a **pas** été récupéré à ce jour — toujours à trancher
>   si la cascade YOLO→crop→CLIP (§ A) en a besoin.
> - Couverture de tests largement étoffée depuis (`go test ./internal/...
>   -race` propre sur tout le dépôt) — le constat "couverture quasi
>   nulle" ci-dessous ne tient plus.
>
> Les décisions H (topologie de déploiement) évoquée en bas de ce document
> a été tranchée le 2026-08-10 (un seul backend Go, deux façades UI via
> Wails) — voir `docs/gui/spec.md` § 0.

---

## Étape 1 — Inventaire des branches

`git fetch --all --prune` a échoué (pas de clé SSH dans cet environnement — `Permission denied (publickey)`). L'inventaire ci-dessous s'appuie sur les refs locales déjà présentes (`remotes/origin/*` à jour au moment du dernier fetch réussi) ; à revalider avec un `git fetch` qui aboutit.

| Branche | Dernier commit | Date | Ahead/behind `main` | Mergée dans `main` |
|---|---|---|---|---|
| `main` | `c959f04` "LIVE-001: Init project wip" | 2025-07-21 | — (référence) | — |
| `realtime-uc` | `13c3026` "move onnx runtime to internal/ dir..." | 2025-07-31 | +4 / -0 | non |
| `feat/ochestrator` | `4c482dc` "tast channel orchestrator" | 2025-08-06 | +10 / -0 | non |
| `feat/displayer` | `2ed56f9` "refacto: migrate vendored go-clean-app to v2.1.0" | 2026-08-04 | +11 / -0 | non |

**`main` est quasiment vide** : il est resté au tout premier commit du projet. Tout le travail réel a été fait sur des branches de feature, jamais réintégrées. C'est la branche la moins fiable pour juger de l'état du projet — ne pas s'y fier (conforme à l'avertissement de la mission sur le README).

**Ancestralité** :
- `realtime-uc` est ancêtre de `feat/displayer` → entièrement absorbée, rien d'unique à en tirer.
- `feat/ochestrator` **n'est PAS ancêtre de `feat/displayer`** : elle diverge au commit `7b21453` ("refacto: vendorize onnx internal") et n'a jamais été mergée. Elle contient un prototype de pipeline par channel (`src/orchestrator/`) absent de la branche actuelle. Détail en 2bis / décision C.

**Recherche de POC yolo/video/gocv** : tout le travail YOLO/gocv/ONNX est concentré sur la lignée `main → realtime-uc → feat/displayer`, pas de branche POC séparée oubliée. En revanche, **deux stashes non commités** existent sur `feat/displayer` et contiennent du travail réel jamais intégré nulle part :

| Stash | Basé sur | Contenu |
|---|---|---|
| `stash@{0}` | `91b1c3f` (2025-08-11) | Implémentation complète **YOLOE11s-seg** (segmentation, `src/implementation/ai/yoloe11s-seg/`, +141 lignes + modèle ONNX 41 Mo), refonte de `AI.AnalyzeFrame(frame, filters)` pour accepter une liste de classes filtrées, refonte de `Render()` pour retourner `(bool, error)` et fusionner la gestion de la touche `Echap` |
| `stash@{1}` | `7b21453` (antérieur) | Version intermédiaire de la refonte `camera_processor.go`, ajout de constantes de classes COCO dans `model/class.go` (remplacées depuis par une autre approche) |

Ce n'est pas du code mort à ignorer : `stash@{0}` en particulier est une implémentation fonctionnelle de la piste "segmentation optionnelle" évoquée en décision A, et une amélioration d'API (`Render` retournant un bool de continuation) plus propre que le code actuel. **Recommandation : le faire relire avant tout travail sur la décision A, il ne doit pas être perdu (un `git stash drop` accidentel le supprimerait définitivement).**

---

## Étape 2 — État des lieux du code réel

### Structure et volumes (branche `feat/displayer`, la plus avancée)

- ~2000 lignes de Go hors `vendor/`, réparties sur ~30 fichiers.
- Pas de `pkg/` ni de `cmd/` à la racine — tout est sous `src/` (`domain/`, `infrastructure/`, `implementation/`, `internal/drawer/`, `transport/`).
- `docs/` existe et contient de la doc réelle (guides, issues, troubleshooting) mais date du tout début du projet (2025-07-19/24) — probablement générée en amont du code, à vérifier avant de s'y fier.
- Tests : 2 fichiers seulement. `domain/dto/dto_test.go` (100% de couverture, mais sur un DTO trivial). `domain/uc/init_test.go` : présent mais **0 test réellement exécuté** (`[no tests to run]`). Couverture effective du projet : quasi nulle.

### Ce qui est réellement implémenté vs déclaré

Le `readme.md` et l'`overview.md` (avant mise à jour de cet audit) décrivaient une architecture cible bien plus riche que l'existant : matching sémantique CLIP en langage naturel, cache LRU, circuit breakers, event-driven, mode batch, persistance, multi-provider IA. **Rien de tout cela n'existe dans le code.** Ce qui existe et fonctionne réellement (testé en conditions réelles, binaire exécuté avec succès) :

- Capture webcam (`gocv`) → détection d'objets YOLO11s en ONNX natif Go → dessin des bounding boxes → affichage fenêtre.
- CLI cobra + mode interactif, tous deux branchés sur le même `UseCases.RecognitionUseCase`.
- Logger structuré zap, shutdown gracieux (juste migré vers `go-clean-app` v2).

### Architecture — sens des dépendances

L'inversion de dépendance est **réelle**, pas cosmétique : `src/domain/uc` n'importe que des interfaces (`infrastructure/ai.AI`, `infrastructure/streamer.{InputStream,OutputStream}`, `infrastructure/notifier.Notifier`) — aucun import direct de `implementation/*` depuis `domain/`. Le câblage concret se fait uniquement dans `main.go`. C'est un vrai port/adapter, même si la nomenclature des dossiers (`infrastructure/` = ports, `implementation/` = adapters) est inversée par rapport aux conventions habituelles.

Deux réserves :
1. `infrastructure/ai.DetectionResult.BoundingBoxes` est typé `[]onnx.BoundingBox` — le type vendorisé de `go-clean-onnxruntime` fuite directement dans le contrat du port, au lieu d'un type domaine propre. Fuite d'abstraction mineure mais réelle.
2. Il n'y a qu'**un seul use case** (`RecognitionUseCase`). Le pattern "use cases indépendants avec leurs propres dépendances" décrit dans `di-architecture-example.md` (ex-`todo.md`) n'a jamais été mis en pratique — ce fichier est un exemple de code générique, pas une description du projet.

### Les transports partagent-ils vraiment la même logique métier ?

Vérifié par grep de `RecognitionUseCase` / `useCases.` dans tous les fichiers de `transport/` :

- **CLI (cobra) et mode interactif** : oui, les deux appellent `useCases.RecognitionUseCase` via `BaseHandler.HandleRecognitionUseCase`.
- **API web (gin) et WebSocket** : **non**. `transport/api/routes.go` (34 lignes) et `transport/websocket/handler.go` (58 lignes) ne contiennent aucun appel à `RecognitionUseCase`. Ce sont des squelettes de routage sans logique métier branchée — confirmé, pas une supposition.

### Hygiène

| Point | Constat |
|---|---|
| `.env` commité | **Confirmé.** Tracké depuis le tout premier commit (`c959f04`), jamais dans `.gitignore` (qui ne contient qu'une ligne : `.roo/`). Le repo est **public** sur GitHub. Contenu actuel sans secret (`APP_NAME`, `APP_VERSION`, `APP_ENV`, `APP_DEBUG` uniquement) — donc pas de fuite de secret *à ce jour*, mais le risque structurel est réel : tant que `.env` reste tracké, toute variable sensible ajoutée un jour finira dans l'historique public de façon permanente. |
| `LICENSE` | **Absent.** Le `readme.md` référence une licence MIT dans un badge mais aucun fichier `LICENSE` n'existe à la racine. |
| Placeholder README | **Confirmé**, `readme.md:5` et `readme.md:48` : `github.com/your-org/livesemantic` — jamais remplacé par l'URL réelle (`github.com/deadelus/live-semantic`). |
| `vendor/` commité | 45 Mo. Se justifie partiellement : deux dépendances (`go-clean-app`, `go-clean-onnxruntime`) sont maintenues par le même auteur et bougent vite (v1→v2 constaté cette semaine), plus des libs cgo (`gocv`) sensibles à l'environnement de build — vendoriser sécurise la reproductibilité. Contrepartie : le `.git` pèse déjà **136 Mo**, en grande partie à cause des binaires committés à plusieurs endroits de l'historique et du stash (`yolo11s.onnx`, `onnxruntime.{so,dylib,dll}` sur plusieurs branches, `yoloe11s-seg.onnx` de 41 Mo dans le stash). Ces binaires devraient être en Git LFS ou téléchargés au build plutôt que committés en dur — ce n'est pas la vendorisation de `vendor/` le problème, ce sont les modèles/libs natifs. |

### Verdict franc

Le projet est un **prototype fonctionnel mono-fonctionnalité**, pas un MVP au sens du roadmap déclaré. Ce qui marche est solide pour ce qu'il fait (build propre, binaire testé bout en bout, DI réelle, transport CLI complet) mais le périmètre réel est étroit : de la détection d'objets YOLO en webcam, point. Aucune des promesses différenciantes du projet (filtres en langage naturel, cascade YOLO→CLIP, tracking, mode batch) n'existe encore en code — tout est en amont, dans des branches non mergées ou des stashes.

**Réutilisable tel quel** : la couche transport CLI/cobra, le pipeline YOLO11s-ONNX-gocv, le système de ports `infrastructure/*`, le shutdown gracieux fraîchement migré. **À ne pas perdre** : le prototype de segmentation dans `stash@{0}`. **À trancher avant de construire dessus** : que faire du prototype `feat/ochestrator` (voir 2bis, décision C) et de l'écart réel vs. annoncé dans le README public.

---

## Étape 2 bis — Matrice de recouvrement décisions ↔ branches

Recherche menée avec `git grep` sur `main`, `feat/displayer`, `feat/ochestrator`, `realtime-uc` (`vendor/` exclu), plus lecture des stashes.

| # | Décision | Statut | Où | Qualité | Action |
|---|---|---|---|---|---|
| A | Cascade YOLO→crop→CLIP + segmentation optionnelle | **ABSENT** (cascade/CLIP) — **PARTIEL** (segmentation) | Segmentation : `stash@{0}` sur `feat/displayer`, `src/implementation/ai/yoloe11s-seg/yoloe11s-seg.go` (141 lignes, implémentation complète du modèle YOLOE11s-seg, jamais commitée) | La partie détection reste solide (même schéma que `yolo11s.go`, éprouvé). Aucune trace de CLIP, de crop de bbox, ni de pipeline en cascade nulle part. | Récupérer et committer `stash@{0}` avant tout (risque de perte). Construire la cascade par-dessus : le crop-vers-CLIP et l'appel CLIP sont entièrement à écrire. |
| B | Tracking-by-detection (KCF/CSRT/MOSSE, IoU) | **ABSENT** (tracker) mais **primitive réutilisable trouvée** | `realtime-uc:src/domain/model/boundingbox.go` avait un vrai type domaine `BoundingBox` avec `.IoU()`, `.Intersection()`, `.Union()` — supprimé au commit `d9f7a96` ("refacto draw box...") lors du déplacement vers la lib vendorisée. La logique IoU **survit** dans `vendor/github.com/deadelus/go-clean-onnxruntime/src/onnx/boundingbox.go`, utilisée en interne pour le NMS (seuil 0.7). | La méthode IoU vendorisée est correcte et testée en usage (NMS), mais elle est privée au package `onnx`, pas exposée comme utilitaire domaine réutilisable pour l'association tracker↔détection. Aucun tracker gocv (KCF/CSRT/MOSSE) n'est utilisé nulle part dans le code applicatif. | Ne pas réécrire l'IoU : soit l'exposer publiquement côté vendor (PR sur `go-clean-onnxruntime`), soit dupliquer les ~10 lignes côté domaine `live-semantic` (plus simple, pas de couplage supplémentaire au vendor). Le tracker lui-même (gocv `NewTrackerKCF`/`CSRT`/`MOSSE`) est entièrement à écrire. |
| C | Découplage async 3 boucles (vidéo/détection/sémantique) | **DIVERGENT** | `feat/ochestrator` (commit unique `4c482dc`, non mergé), `src/orchestrator/orchestrator.go` (90 lignes) + `src/domain/uc/uc_input_webcam.go` + `uc_output_window.go` | Prototype à un seul étage : une seule boucle consommateur (`Input → []Processors séquentiels → Output`), pas trois boucles à fréquences différentes. Le channel `frameChan` (buffer 1, écrasement si plein) respecte bien l'esprit "drop côté détection, jamais côté flux vidéo" — mais ici c'est TOUT le traitement (détection + rendu) qui est dans la même cadence, pas juste la détection. Le code est explicitement un test jetable : `main.go` de cette branche se termine par `panic("Direct access test complete...")`, l'ancien `main()` réel est laissé en commentaire. Non buildable proprement en l'état pour un usage réel. | **Divergent, pas réutilisable tel quel.** Les interfaces (`InputVideoUseCase`, `OutputVideoUseCase`, `FrameProcessorUseCase`) sont un bon point de départ conceptuel et peuvent inspirer les ports de la décision E, mais l'architecture à 1 boucle doit être réécrite en 3 boucles à fréquences séparées. Ne pas merger tel quel — en extraire les idées, pas le code. |
| D | Agrégat `Track` (état Tentative/Confirmed/Coasting/Lost) | **ABSENT** | Aucune occurrence de `TrackID`, `Confirmed`, `Coasting` sur aucune branche. | — | Entièrement à créer, comme prévu. C'est cohérent : la décision D dépend du tracker (B), qui n'existe pas non plus. |
| E | Ports (`VideoSource`, `ObjectDetector`, `ObjectTracker`, `SemanticEncoder`, `AlertSender`, `MetricsCollector`) | **PARTIEL / DIVERGENT** | `feat/displayer` : `infrastructure/ai.AI`, `infrastructure/streamer.{InputStream,OutputStream}`, `infrastructure/notifier.Notifier` (voir étape 2). `feat/ochestrator` : `InputVideoUseCase`/`OutputVideoUseCase`/`FrameProcessorUseCase` (voir décision C). | Le principe d'inversion de dépendance est acquis et fonctionne (étape 2). Mais la forme des ports ne correspond pas à la cible : pas de séparation `ObjectDetector`/`ObjectTracker`/`SemanticEncoder` (tout est mélangé dans `ai.AI.AnalyzeFrame`), pas de `MetricsCollector`. `ai.DetectionResult` fuite un type vendor (voir étape 2). | Réutiliser le principe et le câblage dans `main.go`, mais réécrire les interfaces pour scinder `ai.AI` en `ObjectDetector` + futur `SemanticEncoder`, ajouter `ObjectTracker` et `MetricsCollector` qui n'existent pas. |
| F | ONNX-first, embeddings texte au démarrage, latences réalistes | **PARTIEL** | `implementation/ai/yolo11s/yolo11s.go` | ONNX Go natif est bien le seul backend, trivialement "premier choix" puisque c'est le seul qui existe — l'ordre de préférence n'a pas vraiment été testé (pas de fallback Python/REST à comparer). Aucun texte-encodeur, donc "embeddings calculés une fois au démarrage" ne s'applique à rien pour l'instant. `readme.md`/`overview.md` (avant correction) annonçaient du "sub-50ms" non étayé par un benchmark. | Corriger la doc de latence dès la phase hygiène (fait dans `overview.md` mis à jour, à répercuter dans `readme.md`). Le vrai travail (embeddings CLIP one-shot) dépend de la décision A. |
| G | Restructuration `src/` → `cmd/`+`internal/`, hygiène | **ABSENT** (restructuration) — **problèmes hygiène confirmés** (voir étape 2) | Toutes branches encore en `src/` | — | Voir étape 2 pour le détail hygiène (`.env`, `LICENSE`, placeholder README, binaires committés). La restructuration `cmd/`+`internal/` est un pur renommage/déplacement, faible risque, à faire tôt (phase 1 du plan). |

### Conflits entre branches et point d'arbitrage

- **`feat/ochestrator` vs `feat/displayer`** : conflit réel. Les deux divergent au même commit (`7b21453`) mais évoluent différemment — `feat/displayer` a continué le développement normal (shutdown gracieux, migration go-clean-app v2) pendant que `feat/ochestrator` est parti sur un prototype expérimental jetable jamais fini ni mergé. Le merger tel quel casserait `main.go` (il est actuellement un test qui panique volontairement). **Recommandation : ne pas merger `feat/ochestrator`, en extraire uniquement les idées d'interfaces pour la décision C/E, puis fermer la branche.**
- **`realtime-uc`** : pas de conflit, entièrement absorbée par `feat/displayer`. Peut être supprimée sans perte.
- **`main`** très en retard (0 commit vs 11 sur `feat/displayer`) : un merge direct `feat/displayer → main` ne poserait pas de conflit technique (fast-forward possible), mais tant que ça n'est pas fait, `main` ne reflète rien de l'état réel du projet — quiconque clone `main` obtient le tout premier commit.

### Question à l'utilisateur — je ne tranche pas seul

1. **`feat/ochestrator`** : la garder comme référence (non mergée) le temps de réécrire la vraie architecture 3-boucles, ou la supprimer maintenant que son contenu est documenté ici ?
2. **`realtime-uc`** : à supprimer (entièrement absorbée par `feat/displayer`) ?
3. **`main`** : faire un fast-forward de `feat/displayer` vers `main` maintenant (aucun conflit attendu), ou attendre la phase 1 du plan (hygiène/restructuration) pour ne merger qu'une base propre ?
4. **Les stashes** (`stash@{0}`, `stash@{1}`) : `stash@{0}` contient du travail exploitable (segmentation) — le committer sur une branche dédiée avant de continuer, pour ne pas risquer de le perdre ? `stash@{1}` semble entièrement superseded par `stash@{0}`, à confirmer avant de le `drop`.

### Décision H (topologie de déploiement) — signalée, non tranchée

Rien dans le code actuel ne présage d'un choix : aucune trace de `FrameIngress`, gRPC, MQTT, ni de logique liée à un déploiement multi-nœud. Le code actuel suppose implicitement une machine unique avec caméra locale (`gocv.VideoCapture`). Cette question reste entièrement ouverte et n'a pas été tranchée dans cet audit, conformément à la consigne.
