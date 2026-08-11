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

## 12. Décision inversée : filtrage par label COCO, plus de gate CLIP (2026-08-11)

Suite directe de § 10 : le défaut abaissé à 0.20 a bien rattrapé les vrais positifs manqués, mais a aussi laissé passer des confondants (ex. plante en arrière-plan scorant "person") — attendu, pas une surprise, § 7 avait déjà mesuré un crop "couch" à **0.2476** pour "person", au-dessus de 0.20 *et* de l'ancien 0.25. Les plages de score se chevauchent réellement (vrais "person" ~0.225-0.29, confondants ~0.20-0.2476+) : **aucune valeur de seuil absolu ne sépare proprement les deux**, confirmant l'ouverture laissée en § 7 ("piste plus robuste... pas fait").

Deux options posées à l'utilisateur : (1) score relatif contre un/des prompt(s) négatif(s) générique(s), ou (2) filtrage exact par label YOLO. **Choix explicite : (2), pas (1)** — l'utilisateur ne veut pas de prompt négatif.

**Décision : `reanchor()` (`internal/application/uc/tracking.go`) filtre désormais directement sur `box.Label` (le label YOLO), plus aucun appel CLIP sur ce chemin.** Ça **inverse** la décision "option a" du 2026-08-10 (§ ci-dessus, TODO.md § A) où CLIP décidait seul et le matching par label COCO avait été explicitement supprimé.

**Nouvelle syntaxe de filtre** (`internal/application/uc/filter_spec.go`, `parseFilterSpec`) — demandée précisément par l'utilisateur, pas devinée :
- `person` → jusqu'à 1 track "person" (plafond implicite).
- `person*2` → jusqu'à 2.
- `person*2,car` → plusieurs termes indépendants, un plafond chacun.
- Labels dupliqués entre termes rejetés à l'analyse (erreur, pas un silence) — l'utilisateur veut qu'un futur système d'événements/actions (pas construit, TODO.md § A dernier item) ne se déclenche jamais deux fois pour la même box : avec un matching par label exact, une box n'a qu'un seul label, donc un terme par label suffit à garantir l'absence de chevauchement par construction.
- Chaque label validé contre les 80 classes COCO à l'analyse — un typo (`"perso"`) devient une erreur claire, pas un filtre qui ne matche jamais rien silencieusement (piège déjà rencontré une fois sur ce projet avec `normalizeFilter`, TODO.md § F).

**Conséquence assumée, pas un oubli** : plus de filtre texte libre hors vocabulaire COCO (ex. "sac abandonné" n'est plus filtrable par ce chemin — c'était pourtant la raison d'être initiale de CLIP dans ce projet, cf. § 1). `clip.Encoder` reste chargé/câblé (`main.go`, `uc.NewUseCase` exige toujours un `SemanticEncoder` non-nil) mais n'est plus appelé dans le chemin de matching — gardé pour la feature "galerie de références par image" (TODO.md § D, `EncodeImage` seul, pas `EncodeText`/score).

`dto.RecognitionRequest.SimilarityThreshold` supprimé (devenu inutile) — CLI, cmd, API et tests mis à jour.

**Testé en conditions réelles** (webcam, `--web`, filtre `person*2`) : track "person" confirmé en continu (`state: Confirmed` sur des dizaines de cycles reanchor consécutifs), `active_tracks` ne reste plus jamais bloqué à 0 comme lors du run initial (§ 10) sur la même scène/même personne. Bug annexe trouvé et corrigé au passage : `RecognitionUseCase` ouvrait la caméra *avant* de valider le filtre — un filtre invalide (typo) faisait quand même clignoter/ouvrir la webcam ~800ms pour rien avant d'échouer. Réordonné (`uc_recognition.go`) : validation du filtre en premier.

**Limite qui reste ouverte, pas résolue par ce changement** : le `*N` est pour l'instant un plafond pur (n'accepte pas plus de N tracks). L'intention à terme de l'utilisateur est qu'une condition de scène (ex. "2 personnes simultanément") puisse déclencher une ou plusieurs actions/événements configurables — rien de conçu (ni le modèle d'action, ni la syntaxe pour l'associer à un terme), TODO.md § A dernier item.

## 13. Retour en arrière partiel : filtre hybride label + CLIP (2026-08-11, même après-midi)

Suite directe de § 12, quelques heures plus tard. L'utilisateur reprend : CLIP "était utile quand même" — la demande n'était pas de le supprimer, mais de ne plus l'avoir comme *seul* mécanisme, avec un défaut fixe non exposé (la GUI, § H, le remplacera par un vrai contrôle un jour). Spec donnée explicitement par l'utilisateur, pas devinée :

> si je mets `"person with a red hat"*1` → YOLO propose des candidats, CLIP les score, on affiche 1 seule box (le "bruit" au-delà du plafond pourra déclencher une action ou non, pas construit) ; si je mets juste `person`, le seuil reste par défaut et **n'est pas appliqué** — c'est une classe COCO, elle sait ce qu'elle cherche.

**Décision : `reanchor()` tourne en deux passes.**
- **Pass 1 (exacte)** : un terme qui est une des 80 classes COCO matche directement sur `box.Label` — inchangé vs § 12, toujours aucun appel CLIP.
- **Pass 2 (sémantique)** : un terme qui n'est *pas* une classe COCO est scoré via CLIP contre chaque candidat **non déjà réclamé par la pass 1**, filtré à `defaultSimilarityThreshold` (constante cachée, `0.20` — même valeur que le défaut abaissé en § 10, pas re-calibrée spécifiquement pour ce nouvel usage), **classé par score décroissant**, seuls les `N` meilleurs (le plafond du terme) sont retenus. Le plafond devient donc un vrai top-N pour un terme sémantique — c'est ce qui rend `"..."*1` réellement "1 seule box, la meilleure", pas juste "la première trouvée".

`filterTerm.Label` renommé `Key` (peut être un label exact ou du texte libre). `parseFilterSpec` **n'erreure plus sur un terme hors COCO** — c'est désormais un terme sémantique valide, pas un typo (le typo-protection de § 12 disparaît de facto pour les termes non-COCO ; un vrai typo de "person" devient un filtre sémantique bizarre et probablement peu discriminant plutôt qu'une erreur claire — compromis accepté, pas re-questionné). `dto.RecognitionRequest.SimilarityThreshold` reste supprimé (toujours pas un champ exposé) — le seuil sémantique est une constante interne, pas un paramètre de requête.

**Limite connue, documentée, pas résolue** : un terme exact et un terme sémantique visant le **même label YOLO sous-jacent** (ex. `person*1,person avec quelque chose*1`, les deux `person`-labellisés par YOLO) interagissent mal — la pass 1 réclame toute box labellisée "person" avant que la pass 2 ne les voie, plafond ou pas ; une box qui dépasse le plafond du terme exact ne "retombe" jamais vers le terme sémantique pour une seconde chance. Documenté dans le doc-comment de `reanchor()`, pas contourné — nécessiterait de repenser l'ordre des passes ou de fusionner les deux dans une seule logique de priorité, pas fait.

**Testé en conditions réelles** (webcam, `--web`, filtre `person*1,person with a red hat*1`) : aucune personne dans le cadre au moment du test → le terme exact ne matche rien. Le terme sémantique, lui, matche — une **plante en pot** (`class: potted plant`, `state: Confirmed` en continu, plafond=1 respecté, aucune erreur). Résultat à double lecture : (1) **confirme mécaniquement que le pipeline hybride fonctionne** — partition exact/sémantique correcte, classement par score opérant, plafond respecté ; (2) **confirme aussi, sans le vouloir, que la fragilité documentée en § 7/§ 10 est toujours bien réelle** — maintenant visible sur le chemin sémantique plutôt que sur un ancien gate global unique. Pas une régression du changement de cette section : c'est la même limite de fond que § 12 a déjà actée comme non résolue, juste réapparue dans le nouveau contexte hybride au premier vrai test.

**Questions soulevées par l'utilisateur, pas actées en code** :
- **Auto-calibration** : les futures actions (§ système d'événements, TODO.md § A, pas construit) pourraient ajuster `defaultSimilarityThreshold` automatiquement selon le "bruit" observé — aucune métrique de bruit définie, aucune fenêtre d'observation choisie, dépend du système d'actions pour avoir un point d'accroche.
- **`box.Confidence` (YOLO) et score CLIP sont-ils normalisables ?** Non, pas numériquement — grandeurs de nature différente (probabilité de classification fermée à 80 classes, plage ~0.5-1.0, vs similarité cosinus dans un espace joint image/texte, plage réelle ~0.19-0.29). Les fusionner par min-max ou z-score serait arbitraire. Ce qui est réellement unifiable, c'est le **concept UX** — un seuil de confiance par terme, calculé différemment selon que le terme est exact ou sémantique — pas le calcul lui-même. `thresholdConfidence` (YOLO, `yolo11s.go`, 0.5) reste une constante globale non paramétrable par terme ; `defaultSimilarityThreshold` (CLIP) est déjà conceptuellement "par terme" côté code (`termMatch` est par terme) mais pas encore exposé comme tel. Piste pour plus tard, pas commencée.

## 14. Limite structurelle connue : CLIP et le "binding" attribut-objet (contexte, 2026-08-11)

Question posée par l'utilisateur après § 13 : CLIP est-il seulement entraîné pour matcher un nom (COCO-like) ou peut-il vraiment comprendre une description composée comme `"person with a red hat"` ? Réponse vérifiée par recherche, pas supposée.

**CLIP est entraîné sur de vraies légendes en langage naturel** (~400M paires image/texte, pas des labels à un mot) — une phrase composée n'est donc pas hors-distribution pour lui. Mais la littérature documente un défaut connu, **toujours ouvert en 2025** : CLIP se comporte comme un modèle **"bag-of-words" côté cross-modal** — il détecte les concepts individuels ("person", "red", "hat") mais échoue souvent à lier correctement un attribut à son objet dans une scène composite (exemple canonique : confond "a yellow submarine and a blue bus" avec "a blue submarine and a yellow bus" — mêmes mots, mauvais binding). Un travail récent (ICLR 2026, *CLIP Behaves like a Bag-of-Words Model Cross-modally but not Uni-modally*) précise que l'information de binding existe bien séparément dans les embeddings texte et image, mais se perd spécifiquement dans l'**alignement cross-modal** — pas un manque de connaissance, un défaut structurel de l'entraînement contrastif lui-même. Tentatives de correction (négatifs durs à l'entraînement, changements d'architecture) qui n'ont pas résolu le problème en entier (*Common Data Properties Limit Object-Attribute Binding in CLIP*, 2025).

**Implication pour ce projet, pas mesurée empiriquement ici (hypothèse informée, pas un test contrôlé)** : le ViT-B/32 utilisé (§ 2) est la variante la plus petite (patchs 32×32, moins de résolution spatiale fine que ViT-L/14) — probablement encore moins armée pour un détail composé comme "un chapeau" dans un crop YOLO déjà imprécis (§ 9). Combiné aux marges déjà faibles mesurées ici (0.01-0.03, § 7), une requête composée a vraisemblablement **moins** de pouvoir discriminant qu'un nom simple, pas plus — plus de place pour qu'un confondant se glisse sous le seuil. Le résultat "plante" de § 13 est cohérent avec ça mais ne le prouve pas isolément (aucune personne n'était dans le cadre pendant ce test — impossible de distinguer "CLIP confond les attributs" de "rien ne matchait, le bruit a gagné par défaut"). **Pas testé** : comparer directement une personne avec chapeau rouge vs sans, dans le même cadre, pour isoler l'effet.

## 15. Références

- Modèle original : [openai/clip-vit-base-patch32](https://huggingface.co/openai/clip-vit-base-patch32) (licence MIT, [openai/CLIP](https://github.com/openai/CLIP))
- Export ONNX retenu : [Xenova/clip-vit-base-patch32](https://huggingface.co/Xenova/clip-vit-base-patch32)
- Tokenizer de référence : `openai/CLIP` → `clip/simple_tokenizer.py` (BPE, vocab 49408 tokens)
