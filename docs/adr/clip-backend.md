# Backend CLIP pour la cascade YOLO → crop → CLIP (item A)

> **Statut** : référence technique + ADR
> **Portée** : choix du modèle CLIP, format ONNX, et stratégie de tokenisation texte pour `infrastructure/inference.SemanticEncoder`
> **Dernière révision** : 2026-08-10

---

## 1. Contexte

`infrastructure/inference.SemanticEncoder` (port) existe déjà (`EncodeImage`/`EncodeText` → `entities.Embedding`), sans implémentation. Segmentation `yoloe11s-seg` volontairement pas portée (TODO.md § A, décision du 2026-08-10) — le vrai travail restant est CLIP.

## 2. Modèle retenu

**`openai/clip-vit-base-patch32`**, via l'export ONNX de **Xenova** (`Xenova/clip-vit-base-patch32`, HuggingFace) — pas d'export maison nécessaire, vérifié disponible :

| Fichier | Rôle | Taille (fp32) | Taille (quantized) |
|---|---|---|---|
| `onnx/vision_model.onnx` | Encodeur image → embedding | 335 Mo | 85 Mo |
| `onnx/text_model.onnx` | Encodeur texte → embedding | 242 Mo | 61.5 Mo |
| **Total** | | **577 Mo** | **~146.5 Mo** |

Tailles mesurées directement (`curl -sIL`, `Content-Length` réel après redirection LFS), pas des estimations. Séparation vision/texte en deux fichiers ONNX distincts — correspond exactement au découpage `EncodeImage`/`EncodeText` du port, pas d'adaptation d'architecture nécessaire.

**Licence** : le modèle CLIP original (`openai/CLIP` sur GitHub) est **MIT** (vérifié directement sur le fichier `LICENSE` du repo), compatible avec la licence du projet. Xenova ne fait qu'une conversion de format des mêmes poids publics — pas de changement de licence introduit par la conversion en soi (à revérifier si Xenova ajoute des conditions dans son propre README au moment de l'intégration, pas fait ici).

**Xenova** : organisation HuggingFace établie, maintient les modèles ONNX qui alimentent `transformers.js` (bibliothèque JS largement utilisée) — 130k+ téléchargements sur ce repo précis, mise à jour 2025-07-08, pas un mirror obscur.

## 3. fp32 vs quantized — décision à confirmer

Le projet vise du local-first léger (cf. `readme.md`, `docs/adr/inference-runtimes.md`). `yolo11s.onnx` pèse 38 Mo ; ajouter 577 Mo pour CLIP changerait significativement le poids de `assets/models/` (déjà non versionné dans git depuis la décision `.gitignore` sur les `.onnx`, donc l'impact n'est pas sur la taille du repo, mais sur le temps de téléchargement/setup initial et l'empreinte disque locale).

**Recommandation : la version quantized (~146.5 Mo total)**, cohérente avec l'esprit "léger" du projet. Perte de précision généralement faible pour CLIP en quantization int8 (usage courant en production, notamment ce sont les poids par défaut utilisés par `transformers.js` côté navigateur) — pas mesurée sur ce projet spécifiquement, à valider une fois intégré (rejoint l'item F "benchmarker CLIP" déjà dans la TODO).

## 4. Tokenizer texte — le vrai risque technique

CLIP utilise un tokenizer **BPE (Byte-Pair Encoding) de style GPT-2**, vocabulaire fixe de 49408 tokens, fourni par `vocab.json` + `merges.txt` (présents dans le repo HuggingFace, ~350 Ko pour `merges.txt`). **Aucune bibliothèque Go officielle ou largement adoptée pour ça** — recherché : `sugarme/tokenizer` existe et supporte BPE générique depuis fichiers vocab/merges, mais statut de maintenance incertain (pas de date de dernière release trouvée, pas de mention explicite de compatibilité CLIP).

**Décision proposée** : écrire notre propre implémentation BPE minimale, dépendance-free, plutôt que d'importer une lib tierce d'entretien incertain pour une pièce aussi critique (correction du texte = correction des filtres en langage naturel, le cœur de la promesse du projet). L'algorithme est stable et documenté depuis 2021 (`openai/CLIP`, `simple_tokenizer.py`), le vocabulaire ne change pas — un port direct de cette logique en Go est un travail borné, pas un risque ouvert. Cohérent avec la prudence déjà démontrée sur les dépendances de ce projet (vendoring, ADRs runtime).

## 5. Plan d'intégration (schéma déjà éprouvé deux fois)

Nouveau package `internal/implementation/inference/onnx/clip/`, même patron que `yolo11s/yolo11s.go` (déjà utilisé pour YOLO11s, et avant refonte pour YOLOE11s-seg) :
- `clip.go` : `Encoder` implémentant `infrastructure/inference.SemanticEncoder`, deux sessions ONNX (`onnxruntime_go`, comme YOLO — pas de nouvelle lib runtime).
- `tokenizer.go` : BPE minimal (vocab.json/merges.txt embarqués ou chargés depuis `assets/`).
- Embeddings texte des filtres calculés une fois au démarrage (déjà planifié, TODO.md § A).
- Crop de la frame sur la bbox YOLO avant `EncodeImage` (déjà planifié, dépend du tracker — fait, § B).

**Ordre décidé avec l'utilisateur (2026-08-10)** : filtre texte d'abord (ce plan), reconnaissance par référence image ensuite (TODO.md § D, nouvel item) — sélection d'une box en direct + label → `EncodeImage` sur le crop → galerie locale `{label, embedding}`, comparaison par similarité cosinus. N'utilise que `EncodeImage` (pas de tokenizer nécessaire pour ce mode), mais demande une UI d'interaction pendant que le flux tourne, pas encore conçue. Le garder en tête en concevant `Encoder` : les deux modes doivent pouvoir réutiliser la même méthode `EncodeImage`, pas de couplage à prévoir spécifiquement au texte.

## 6. Vérifié en conditions réelles le 2026-08-10

Poids quantized téléchargés (`assets/models/clip/{vision,text}_model.onnx`, 89 Mo + 64.5 Mo), premier run réel :

- **`attention_mask` n'existe pas sur ce graphe** — ORT rejette avec `Invalid input name: attention_mask`. Corrigé : `text_model.onnx` prend seulement `input_ids`. Cohérent avec le design original de CLIP (pooling par position du token eot, pas de masque explicite nécessaire).
- Compatibilité opset avec `onnxruntime_go`/lib bundlée (1.22.0) : OK, chargement et inférence sans erreur.
- `EncodeText` : normes L2 = 1.0000 (normalisation correcte). Similarité texte-texte élevée (~0.94-0.95 entre "a photo of a dog/cat/car") — attendu, le préfixe partagé "a photo of a" domine sur des prompts courts, pas un signe de bug.
- `EncodeImage` sur une vraie frame (non recadrée — test rapide, pas encore le pipeline final avec crop YOLO) : ordre de similarité correct (`person` 0.2165 > `car` 0.2130 > `cat` 0.1926) mais marges faibles. Attendu : le crop sur la bbox YOLO (prévu, pas encore câblé) devrait nettement renforcer le signal en retirant le fond de la frame.
- **Crash SIGABRT à la sortie reproduit** (même bug que celui corrigé pour `yolo11s.Detector`, TODO.md bug critique) — `clip.Encoder.Cleanup()` n'appelait pas `runtime.DestroyEnvironment()`. Corrigé de la même façon. 3/3 sorties propres après fix.
- Précision réelle de la version quantized vs fp32 : pas comparée, benchmark à faire (TODO.md § F).

## 7. Calibration du seuil de similarité (2026-08-10)

Test end-to-end réel (webcam + crop YOLO + `EncodeImage`, TODO.md § A) suivi d'un isolement du problème via un outil jetable (`cmd/clip-debug`, supprimé après usage — les résultats sont consignés ici).

**Constat** : à `similarity-threshold: 0.8` (défaut hérité de l'ancien `box.Confidence` YOLO), le filtre rejette silencieusement 100% des détections — les scores CLIP réels ne dépassent jamais ~0.28 dans ces tests. Défaut changé à `0.25` (`internal/transport/adapters/cli/cli_recognition.go`, `internal/transport/adapters/cmd/cmd_realtime_analysis.go`).

**Mesures** (webcam, crops réels 8 échantillons, filtre "person") :
- Plage globale des scores : 0.216 à 0.242 (étalement de seulement 0.026) — très resserré.
- Confondants observés : un crop "couch" (fond de canapé, YOLO mal étiqueté) a obtenu un score plus élevé pour "person" (0.2476) et pour "person" que pour "couch" (0.2393) lui-même. Un crop "person" au cadrage large (fond canapé majoritaire dans la bbox) a scoré plus haut pour "couch" (0.2456-0.2594) que pour "person" (0.2252-0.2290).

**Contre-test avec une image de référence propre** (photo COCO haute résolution, deux chats sur un canapé, aucun flou/compression webcam) : classement correct — "cat" gagne (0.2563) devant "couch" (0.2437), avec une marge réelle mais modeste (~0.012). **Confirme que le pipeline (crop, prétraitement, encodage, normalisation) n'est pas buggé** — le problème est réel mais spécifique aux crops webcam : bbox YOLO peu précises (beaucoup de fond dans la boîte, cf. `crop-2-person.png` dans les logs de session) + frames basse résolution/floues compriment encore plus les marges déjà faibles du zero-shot CLIP.

**Implication architecturale** : un seuil absolu fixe est intrinsèquement fragile pour ce modèle — même sur une image propre, l'écart gagnant/second est de l'ordre de 0.01-0.03, pas une marge confortable. `0.25` est un point de départ défendable (empiriquement, aucun vrai score observé ne dépasse ~0.28, aucun score "bruit" ne descend sous ~0.19), pas une valeur validée rigoureusement pour tous les cas d'usage.

**Piste non explorée, pour plus tard (TODO.md § A/F)** : la pratique standard CLIP zero-shot n'est pas "score ≥ seuil fixe" mais un score **relatif** entre plusieurs prompts candidats (softmax/argmax sur un ensemble de classes). Avec un seul filtre texte libre, il n'y a pas d'ensemble de classes à comparer — mais on pourrait comparer contre un prompt "négatif" générique (ex. "background", "something else") plutôt qu'un seuil absolu, ce qui serait probablement plus robuste que la calibration actuelle. Pas fait dans cette passe.

## 8. Benchmark de latence (2026-08-10, TODO.md § F)

CPU ARM (Apple Silicon, seul matériel disponible dans cette session — x86 non testé, à refaire si besoin). Outil jetable `cmd/perf-bench` (supprimé après usage), image fixe hors webcam (COCO, 640x480, 3 boîtes détectées : 2 "cat" + 1 "remote"), poids quantized, n=30 par mesure, après un appel de warm-up (coûts d'init paresseuse d'ORT/session écartés) :

| Étape | Moyenne | Min-Max |
|---|---|---|
| `yolo11s.Detector.AnalyzeFrame` | ~182ms | 154-260ms |
| `entities.Frame.Crop` | ~9µs | 1-48µs (zero-copy confirmé, négligeable) |
| `clip.Encoder.EncodeImage` (par boîte) | ~46ms | 36-68ms |
| `clip.Encoder.EncodeText` (1× par filtre, pas sur le chemin chaud) | ~23ms | 19-28ms |

**YOLO reste le poste dominant, pas CLIP** — contrairement à une hypothèse non vérifiée avancée plus tôt dans cette session (TODO.md § A, item "piste écartée YOLO plus léger", corrigé). Le coût CLIP cumulé (N × ~46ms) ne dépasse celui de YOLO qu'à partir de ~4 boîtes candidates détectées sur une même frame. Estimation `reanchor()` à 3 boîtes : 182 + 3×46 ≈ 320ms, cohérent avec les 230-390ms mesurés en webcam réelle (§ 7).

## 9. Bbox trop larges — piste "inset" testée et écartée (2026-08-10)

Suite à § 7 (confondants "couch"/"person" attribués en partie à des bbox incluant trop de fond), hypothèse testée directement plutôt que supposée : rétrécir systématiquement chaque bbox YOLO d'une marge fixe avant crop devrait réduire la dilution du signal CLIP par le fond.

Outil jetable `cmd/crop-inset-test` (supprimé après usage) : une frame webcam réelle par run, YOLO détecte, pour chaque boîte on compare CLIP(crop brut) vs CLIP(crop rétréci de 10/20/30% par côté) contre le texte de son propre label COCO. 8 runs, 9 échantillons "person", 5 "couch" (le contenu réel de la frame variait d'un run à l'autre, pas contrôlé).

**Résultat, cohérent mais pas généralisable** :

| Classe | Marge | Delta moyen | Cohérence |
|---|---|---|---|
| person | 20% | +0.010 | 8/9 positifs |
| person | 30% | +0.014 | 9/9 positifs |
| couch | 20% | -0.015 | 5/5 négatifs |
| couch | 30% | -0.029 | 5/5 négatifs, empire avec la marge |

Rétrécir aide un objet compact/premier-plan (moins de fond dans la boîte = signal plus pur) mais dégrade un objet large/plat dont l'identité visuelle dépend de sa silhouette entière (rogner un canapé sur les bords retire l'info qui le distingue d'un simple gros plan de tissu).

**Décision : pas de fix implémenté.** Un inset universel améliorerait certains filtres et en dégraderait d'autres silencieusement — pire qu'un statu quo documenté. Conditionner la marge sur le label COCO réintroduirait une dépendance à la classification fermée que la décision (a) (CLIP décide seul, TODO.md § A) visait explicitement à éliminer. Piste restant ouverte pour plus tard : une mesure indépendante de la classe (ex. variance de couleur/texture dans la boîte, ou un score de "confiance de bord" si le detector l'exposait) plutôt qu'un pourcentage fixe — pas creusé.

## 10. Défaut abaissé 0.25 → 0.20 après un vrai run bloqué (2026-08-11)

Signalé par l'utilisateur : session interactive réelle (`livesemantic -i`, webcam, filtre "person", seuil par défaut 0.25) — **aucune détection sur tout le run** (`active_tracks: 0` sur ~30 cycles de reanchor). Coïncidait avec la migration `DynamicAdvancedSession` de la veille (TODO.md § H1, commit `596d4c2`) — première hypothèse : régression de cette migration.

**Écartée, pas supposée.** Outil de debug jetable (`cmd/debug-migration`, supprimé après usage) : YOLO + CLIP isolés de la webcam, testés sur `assets/videos/person.mp4`.

- **YOLO fonctionne** : détecte "person" de façon fiable à partir de la frame 5 (0-4 sans personne visible dans le cadre, comme le fait déjà remarquer § 9/`cmd/tracking-drift-bench` pour `car.mp4` — pas toutes les vidéos ont la cible dès la frame 0).
- **CLIP donne un score cohérent mais insuffisant** : 0.2351 et 0.2381 sur les deux boîtes "person" détectées (frame 5) — **sous le seuil par défaut 0.25**.
- **Confirmation que ce n'est pas la migration** : mêmes deux crops rejoués sur le commit juste avant `596d4c2` (`AdvancedSession`, ancien pattern) → **résultat rigoureusement identique** (0.2351 / 0.2381, aux 4 décimales près). La migration n'a strictement rien changé numériquement, comme attendu (elle ne touche que l'allocation des tenseurs, pas le calcul).

**Root cause réelle : § 7 se confirme en conditions réelles**, pas juste sur les crops webcam dégradés du run initial — même sur une vidéo de test propre (`person.mp4`, cadrage correct, pas de flou), le score d'un vrai match "person" (0.235-0.238) tombe sous 0.25. La marge documentée en § 7 (0.01-0.03 entre premier et second choix, y compris sur image propre) n'était pas qu'un cas limite isolé — 0.25 rejetait des détections correctes, pas seulement du bruit.

**Décision : défaut abaissé à 0.20** (`cli_recognition.go`, `cmd_realtime_analysis.go`) — toujours au-dessus du plancher de bruit empirique (~0.19, § 7), sous la marge des vrais matches mesurés ici (0.235-0.238). Point ouvert de § 7 toujours valable et non résolu par ce changement : un seuil absolu reste intrinsèquement fragile quelle que soit sa valeur — la piste "score relatif contre un prompt négatif" (softmax sur un ensemble, pas un seuil isolé) reste la vraie solution à long terme, pas explorée.

## 11. Références

- Modèle original : [openai/clip-vit-base-patch32](https://huggingface.co/openai/clip-vit-base-patch32) (licence MIT, [openai/CLIP](https://github.com/openai/CLIP))
- Export ONNX retenu : [Xenova/clip-vit-base-patch32](https://huggingface.co/Xenova/clip-vit-base-patch32)
- Tokenizer de référence : `openai/CLIP` → `clip/simple_tokenizer.py` (BPE, vocab 49408 tokens)
