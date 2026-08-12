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

**Comportement par défaut, confirmé — pas une limite à corriger (reformulé après coup, l'entrée précédente de cette section le présentait comme une limite à lever).** Un terme exact et un terme sémantique visant le **même label YOLO sous-jacent** (ex. `person*1,person with a yellow hat*1`, les deux `person`-labellisés par YOLO) ne se partagent jamais une box : la pass 1 réclame toute box labellisée "person" avant que la pass 2 ne la voie. Testé en direct (webcam, l'utilisateur seul devant la caméra) puis **confirmé explicitement comme le comportement voulu** : *"je ne veux pas deux box sur moi, à moins qu'on nomme un paramètre d'overlaping pour ça"* — pas de double box sur la même personne par défaut, sauf si un paramètre `overlap` nommé (pas construit, TODO.md § A) l'autorise explicitement un jour.

**Testé en conditions réelles** (webcam, `--web`, filtre `person*1,person with a red hat*1`) : aucune personne dans le cadre au moment du test → le terme exact ne matche rien. Le terme sémantique, lui, matche — une **plante en pot** (`class: potted plant`, `state: Confirmed` en continu, plafond=1 respecté, aucune erreur). Résultat à double lecture : (1) **confirme mécaniquement que le pipeline hybride fonctionne** — partition exact/sémantique correcte, classement par score opérant, plafond respecté ; (2) **confirme aussi, sans le vouloir, que la fragilité documentée en § 7/§ 10 est toujours bien réelle** — maintenant visible sur le chemin sémantique plutôt que sur un ancien gate global unique. Pas une régression du changement de cette section : c'est la même limite de fond que § 12 a déjà actée comme non résolue, juste réapparue dans le nouveau contexte hybride au premier vrai test.

**Questions soulevées par l'utilisateur, pas actées en code** :
- **Auto-calibration** : les futures actions (§ système d'événements, TODO.md § A, pas construit) pourraient ajuster `defaultSimilarityThreshold` automatiquement selon le "bruit" observé — aucune métrique de bruit définie, aucune fenêtre d'observation choisie, dépend du système d'actions pour avoir un point d'accroche.
- **`box.Confidence` (YOLO) et score CLIP sont-ils normalisables ?** Non, pas numériquement — grandeurs de nature différente (probabilité de classification fermée à 80 classes, plage ~0.5-1.0, vs similarité cosinus dans un espace joint image/texte, plage réelle ~0.19-0.29). Les fusionner par min-max ou z-score serait arbitraire. Ce qui est réellement unifiable, c'est le **concept UX** — un seuil de confiance par terme, calculé différemment selon que le terme est exact ou sémantique — pas le calcul lui-même. `thresholdConfidence` (YOLO, `yolo11s.go`, 0.5) reste une constante globale non paramétrable par terme ; `defaultSimilarityThreshold` (CLIP) est déjà conceptuellement "par terme" côté code (`termMatch` est par terme) mais pas encore exposé comme tel. Piste pour plus tard, pas commencée.

## 14. Limite structurelle connue : CLIP et le "binding" attribut-objet (contexte, 2026-08-11)

Question posée par l'utilisateur après § 13 : CLIP est-il seulement entraîné pour matcher un nom (COCO-like) ou peut-il vraiment comprendre une description composée comme `"person with a red hat"` ? Réponse vérifiée par recherche, pas supposée.

**CLIP est entraîné sur de vraies légendes en langage naturel** (~400M paires image/texte, pas des labels à un mot) — une phrase composée n'est donc pas hors-distribution pour lui. Mais la littérature documente un défaut connu, **toujours ouvert en 2025** : CLIP se comporte comme un modèle **"bag-of-words" côté cross-modal** — il détecte les concepts individuels ("person", "red", "hat") mais échoue souvent à lier correctement un attribut à son objet dans une scène composite (exemple canonique : confond "a yellow submarine and a blue bus" avec "a blue submarine and a yellow bus" — mêmes mots, mauvais binding). Un travail récent (ICLR 2026, *CLIP Behaves like a Bag-of-Words Model Cross-modally but not Uni-modally*) précise que l'information de binding existe bien séparément dans les embeddings texte et image, mais se perd spécifiquement dans l'**alignement cross-modal** — pas un manque de connaissance, un défaut structurel de l'entraînement contrastif lui-même. Tentatives de correction (négatifs durs à l'entraînement, changements d'architecture) qui n'ont pas résolu le problème en entier (*Common Data Properties Limit Object-Attribute Binding in CLIP*, 2025).

**Implication pour ce projet, pas mesurée empiriquement ici (hypothèse informée, pas un test contrôlé)** : le ViT-B/32 utilisé (§ 2) est la variante la plus petite (patchs 32×32, moins de résolution spatiale fine que ViT-L/14) — probablement encore moins armée pour un détail composé comme "un chapeau" dans un crop YOLO déjà imprécis (§ 9). Combiné aux marges déjà faibles mesurées ici (0.01-0.03, § 7), une requête composée a vraisemblablement **moins** de pouvoir discriminant qu'un nom simple, pas plus — plus de place pour qu'un confondant se glisse sous le seuil. Le résultat "plante" de § 13 est cohérent avec ça mais ne le prouve pas isolément (aucune personne n'était dans le cadre pendant ce test — impossible de distinguer "CLIP confond les attributs" de "rien ne matchait, le bruit a gagné par défaut"). **Pas testé** : comparer directement une personne avec chapeau rouge vs sans, dans le même cadre, pour isoler l'effet.

## 15. Deux bugs de shutdown trouvés en testant § 13/14 en réel (2026-08-11)

Pas des bugs CLIP — trouvés par accident en testant le filtre hybride en webcam réelle (`--web`), en reproduisant le geste normal `POST /recognition/stop` puis `pkill` (SIGTERM) peu après. Les deux sont réels, reproduits, corrigés, et re-vérifiés par le même repro exact après coup — pas supposés corrigés.

**1. SIGSEGV réel** : le handler de shutdown gracieux (`main.go`, `engine.Gracefull().Register`) appelait `objectDetector.Cleanup()`/`semanticEncoder.Cleanup()` **sans attendre** qu'une éventuelle session `RecognitionUseCase` en cours ait fini — détruisant la session ONNX CLIP partagée pendant qu'une goroutine de détection était encore en plein `EncodeImage()` dessus. Crash confirmé, trace complète : `SIGSEGV` dans `_Cfunc_RunOrtSession` (CGo), appelé depuis `clip.Encoder.EncodeImage` → `tracking.go:365` (le pass 2 sémantique) → `RecognitionUseCase.func2` (la goroutine de détection), concurrent avec `main.go:134` → `clip.Encoder.Cleanup()` déclenché par le SIGTERM. La fenêtre de course est plus large avec un terme sémantique actif (cycles reanchor ~250-600ms avec CLIP, contre ~150-270ms sans) — pas un hasard que ça soit sorti en testant § 13, pas avant.

Fix : `uc.UseCase` a maintenant un `sync.WaitGroup` (`activeSessions`) incrémenté/décrémenté autour de tout le corps de `RecognitionUseCase` (`uc_recognition.go`), exposé via une nouvelle méthode `Wait()` sur l'interface `UseCases`. Le handler de shutdown (`main.go`, déplacé après la construction de `useCases` — il la référence maintenant) appelle `useCases.Stop()` puis `useCases.Wait()` **avant** `objectDetector.Cleanup()`/`semanticEncoder.Cleanup()`. Revérifié avec le repro exact (`stop` → `sleep 1` → `pkill`) : plus de crash, `"Recognition completed successfully."` apparaît bien avant `"Shutting down in progress..."` dans les logs.

**2. Process qui ne quitte jamais** : trouvé en vérifiant le fix ci-dessus — les hooks de shutdown se terminaient bien (`"Shutdown is over."` loggé) mais le process **restait vivant**, port toujours ouvert, seul un `SIGKILL` en venait à bout. Cause : `gin.Engine.Run()` (`api.Server.Start()`) bloque indéfiniment sur `http.ListenAndServe`, sans exposer le `*http.Server` sous-jacent pour appeler `Shutdown(ctx)` — et rien ne forçait la sortie du process une fois les hooks `Gracefull` terminés (`lifecycle.Gracefull.gracefullAll()`, `go-clean-app/v2`, ferme juste un channel `done`, n'appelle jamais `os.Exit`). Probablement présent depuis le tout premier câblage du serveur web (H1, pas remarqué avant faute d'avoir vérifié `ps`/le port après un `pkill` — les tests précédents se fiaient aux logs, pas à l'état réel du process).

Fix : `startWebServer` (`main.go`) lance `server.Start()` dans une goroutine et attend sur un `select` entre son erreur éventuelle et `engine.Gracefull().Done()` — dans ce second cas, `os.Exit(0)` explicite. Pas élégant (pas de `http.Server.Shutdown()` propre faute d'accès au listener via l'API gin utilisée ici) mais correct et vérifié : process et port libérés après le même repro exact.

**Pourquoi ça n'était pas dans § H1 initial** : les tests précédents de câblage API (§ H1, TODO.md) validaient toujours `stop` puis laissaient plusieurs secondes avant `pkill`, ou ne vérifiaient que les logs sans `ps`/`lsof` après coup — la fenêtre de course du bug 1 ne s'est ouverte que sous charge CLIP (cycles plus longs), et le bug 2 était invisible sans vérifier l'état du process après le kill, pas juste ses logs.

## 16. Syntaxe `+option` pour `overlap`, tranchée (2026-08-11 soir)

Suite de § 13 : l'utilisateur avait confirmé vouloir `overlap=false` par défaut partout (pas de double box sur le même objet), avec un paramètre nommé pour l'activer explicitement plus tard — syntaxe laissée ouverte, un exemple jeté (`person*1!overlap`) sans être tranché.

**`!overlap` écarté** : ambigu, `!x` se lit dans beaucoup de conventions (shell, comparateurs) comme une négation, pas une activation — `person*1!overlap` aurait pu se lire "overlap désactivé" plutôt que l'inverse. Pas une bonne base pour un vrai paramètre.

**Grammaire retenue, proposée et validée avec l'utilisateur** :

```
key[*cap][+option[=valeur]]...
```

- `+overlap` seul → `true` implicite (la présence active l'option, cohérent avec `*cap` : présence = valeur définie).
- `+overlap=true` / `+overlap=false` → valeur explicite (utile pour de la génération programmatique côté GUI, pas juste la saisie manuelle).
- Chaînable pour de futures options : `person*2+overlap+futureopt=valeur`.
- Grammaire de nom d'option **stricte**, contrairement au `key` (qui accepte n'importe quel texte libre depuis la décision hybride, § 12-13) : un nom d'option inconnu, une valeur non-booléenne pour `overlap`, ou une option répétée dans le même terme sont des erreurs de parsing, pas des silences — il n'y a pas d'interprétation de repli raisonnable pour un nom d'option, contrairement à un `key` qui peut toujours devenir un terme sémantique.

**Implémenté (`internal/application/uc/filter_spec.go`)** : `filterTerm.Overlap bool`, propagé jusqu'à `termMatch.Overlap` (`tracking.go`) au niveau du store interne de `trackManager`. **Pas encore branché dans `reanchor()`** — la valeur est parsée et stockée mais aucune des deux passes ne la consulte ; le comportement observable reste `overlap=false` partout quel que soit ce qu'on écrit dans le filtre. Câblage réel laissé en TODO.md § A, faute de cas d'usage concret qui en dépende pour l'instant (attend aussi le système d'événements/actions, § A, pour qu'un chevauchement volontaire serve à quelque chose).

11 nouveaux cas de test (`filter_spec_test.go`) : `+overlap` implicite/explicite, options inconnues rejetées, valeurs non-booléennes rejetées, option répétée rejetée, terme sémantique avec option sans cap explicite.

## 17. `Overlap` câblé + restriction par label mentionné + bug de confirmation trouvé (2026-08-11, fin de soirée)

Suite de § 16 : l'utilisateur voulait tester en vrai `person` + `person with a yellow hat` avec overlap actif (deux boxes sur la même personne), et voulait explicitement éliminer le bruit couch/plant relevé en § 13.

**Deux mécanismes ajoutés à `reanchor()` (pass 2) :**

1. **`Overlap` consulté** : `if claimed[i] && !term.Overlap { continue }` — une box déjà réclamée par un autre terme ce cycle reste candidate si le terme sémantique courant a `Overlap=true`. Symétrique et simple à raisonner : chaque terme décide pour lui-même s'il accepte de partager, indépendamment de qui a déjà réclamé la box.

2. **`semanticLabelHint` (`filter_spec.go`)** : si le texte libre d'un terme sémantique mentionne **exactement une** classe COCO comme mot entier (ex. "person" dans "person with a yellow hat", détecté avec vérification de frontière de mot pour éviter qu'un `containsWord` naïf ne matche "car" dans "scary"/"carpet"), les candidats de ce terme sont restreints aux boxes déjà labellisées ainsi par YOLO. Zéro ou 2+ classes mentionnées (ex. "person near a car") → pas de restriction, comportement inchangé (trop ambigu pour deviner laquelle est le vrai sujet). Effet direct : un canapé ou une plante ne sont plus jamais candidats pour une requête qui mentionne "person" — pas seulement moins bien notés, **jamais évalués du tout** (bonus : moins d'appels `EncodeImage` par cycle).

**Bug plus profond trouvé en écrivant le test d'intégration pour `Overlap`, pas spécifique à cette feature.** Premier test écrit (`person*1` exact + `person with a red hat*1+overlap` sémantique sur la même box) échouait : 1 track au lieu de 2. Investigation : `spawn()` ne renseignait jamais `matchedTrackIDs` pour le track qu'il venait de créer. Deux conséquences :
- Dans le cas testé : le terme sémantique, en repassant sur la même box via `matchOrSpawn` → `bestMatch`, "volait" le track fraîchement spawné par le terme exact (même `Class`, IoU=1.0, pas dans `matchedTrackIDs` donc considéré libre) au lieu d'en créer un second.
- **Plus grave et indépendant d'`overlap`** : `missUnmatched` (appelé une fois par cycle, après toutes les passes) itère `m.active` et appelle `Miss()` sur tout track absent de `matchedTrackIDs` — un track tout juste `spawn()`-é en faisait partie, à chaque cycle, pour **tout** filtre, pas seulement les cas `overlap`. `Miss()` remet `hits` à 0 : un track qui vient de naître se faisait donc immédiatement infliger un miss dans le même cycle, retardant sa confirmation d'un cycle supplémentaire à chaque fois (`minHitsToConfirm=3` devenait effectivement 4 cycles), et dans le pire cas (un vrai miss juste après le spawn) pouvait le faire passer direct en `StateLost` (`maxMissesBeforeLost=2`) sans jamais confirmer. Bug présent depuis la création de l'agrégat `Track` (TODO.md § D, 2026-08-09), jamais détecté faute d'un test qui vérifiait le nombre de cycles nécessaires à la confirmation.

**Fix** : `spawn()` renvoie désormais `(id string, ok bool)` ; `matchOrSpawn` ajoute l'id à `matchedTrackIDs` sur un spawn réussi, exactement comme il le fait déjà sur un match. Test de régression dédié (`TestReanchor_SpawnedTrackConfirmsInExactlyMinHitsCycles`) — **vérifié qu'il détecte réellement le bug** : fix temporairement désactivé (une ligne commentée), test repassé, confirmé en échec (`Tentative` au lieu de `Confirmed` après 3 cycles), fix restauré, test repassé au vert. Pas juste écrit et supposé correct.

**Testé en conditions réelles** (webcam, filtre `person*1,person with a yellow hat*1+overlap`) : `track-1` (`class:"person"`, terme exact) et `track-2` (`filterKey:"person with a yellow hat"`, terme sémantique) tous les deux `Confirmed` en continu sur la même personne — le double-box demandé fonctionne. Scores sémantiques systématiquement `yolo_label:"person"` (jamais couch/plant) à ~0.25-0.27 sur une dizaine de cycles consécutifs — la restriction par label élimine bien le bruit sans dégrader le vrai match.

## 18. Bug d'affichage trouvé en testant § 17 à la main (2026-08-11, nuit)

L'utilisateur relance en CLI (`go run ./cmd/livesemantic recognition --filter="person*1,person with a yellow hat*1+overlap"`) pour vérifier visuellement le double-box confirmé par les logs en § 17 — et ne voit **pas** deux boxes.

**Cause : bug de rendu, pas de matching.** `trackManager.boxes()` (consommé par `RecognitionUseCase` pour dessiner, `uc_recognition.go`) renvoyait `entities.BoundingBox` brut — dont le champ `Label` est **celui de YOLO** ("person" pour les deux tracks, puisque les deux sont ancrés sur la même détection physique), pas le terme de filtre qui a fait matcher chaque track. Le code de dessin faisait `drawer.BoxID(box.Label)` → même `id` pour les deux tracks → **même couleur, même texte, mêmes coordonnées** (les deux tracks pointent sur la même box physique) → deux rectangles rigoureusement identiques et superposés, visuellement indiscernables d'un seul. Les deux tracks existaient bel et bien (déjà confirmé par les logs en § 17, `track-1`/`track-2` tous les deux `Confirmed`) — seul l'affichage ne les distinguait pas.

**Fix** : `trackManager.boxes()` renvoie désormais `[]trackedBox{Box, FilterKey, TrackID}` au lieu de `[]entities.BoundingBox` nu. `uc_recognition.go` dessine avec `drawer.BoxID(tb.FilterKey)` — couleur et texte dérivés du terme de filtre (`"person"` vs `"person with a yellow hat"`), plus de collision entre deux tracks sur le même objet physique. Nouveau test (`TestBoxes_DistinguishesTracksSharingTheSamePhysicalBox`) verrouille le contrat : `boxes()` doit renvoyer un `FilterKey` distinct par track même quand `Box.Label` est identique.

**Limite qui reste, pas dans le périmètre de ce fix** : les deux boxes restent **superposées aux mêmes coordonnées** (X1/Y1/X2/Y2 identiques — c'est factuellement correct, les deux tracks suivent le même objet physique) — seules la couleur et l'étiquette texte les distinguent maintenant, pas de décalage visuel entre les rectangles. Pas testé visuellement en conditions réelles à l'instant où ce correctif a été écrit (webcam vide au moment de la vérification, capture de frame via `/ws` sans personne dans le cadre) — à confirmer par l'utilisateur à son prochain test.

## 19. Décalage en cascade des boxes superposées (2026-08-11, nuit)

Suite immédiate de § 18 : couleur/texte distincts ne suffisent pas si les deux boxes restent aux mêmes coordonnées exactes — le texte de l'une reste caché derrière/sur celui de l'autre. Demandé explicitement : les décaler pour que les deux textes soient lisibles.

**`cascadeOffsets` (`tracking.go`)** : pour chaque box, compte combien d'*autres* boxes ont déjà (par ordre de `TrackID`, pas l'ordre de la slice — `trackManager.boxes()` itère une map Go, ordre randomisé à chaque appel) une IoU mutuelle ≥ `cascadeOverlapIoU` (0.85, volontairement proche de 1 pour ne capturer que de vrais doublons sur le même objet physique, pas deux objets différents qui se chevauchent un peu) — ce rang devient un décalage diagonal de `cascadeStepPx` (16px) par cran, appliqué aux 4 coordonnées de la box avant dessin. Tri par `TrackID` explicitement pour la stabilité : sans ça, l'ordre aléatoire de la map ferait "sauter" le décalage d'une frame à l'autre pour la même paire de tracks — visuellement, un scintillement au lieu d'un décalage stable.

5 tests (`TestCascadeOffsets`) : box seule, boxes non chevauchantes, 2/3 boxes identiques (décalage incrémental), stabilité indépendante de l'ordre de la slice d'entrée.

**Vérifié visuellement** (webcam réelle cette fois, capture d'une frame via `/ws`) : deux rectangles décalés en diagonale, l'un rouge "person (86.35%)", l'autre gris "person with a yellow hat (86.35%)", les deux textes lisibles simultanément. Confirme que le fix § 18 (identité par `FilterKey`) et ce décalage fonctionnent ensemble comme prévu.

## 20. Score affiché trompeur — corrigé (2026-08-11, nuit)

L'utilisateur relance § 19 sans casquette jaune (pas de "yellow hat") et voit quand même 2 boxes, toutes les deux à "85.37%". Deux constats distincts :

1. **Attendu, pas un bug** : ça confirme en conditions réelles et contrôlées la fragilité déjà documentée (§ 7, § 14 "bag-of-words") — CLIP matche "person with a yellow hat" sur n'importe quelle personne, casquette ou pas. Cette fois le test est propre (le terme exact "person" confirme qu'une personne est bien présente, donc ce n'est pas juste "rien dans le cadre, le bruit gagne par défaut" comme en § 13) : c'est bien le terme sémantique qui ne discrimine pas l'attribut.

2. **Bug réel, distinct** : les deux boxes affichaient le **même** "85.37%" — qui s'avère être `box.Confidence` (la confiance de détection **YOLO**, générique, pas liée à CLIP) recopiée sur les deux tracks, y compris le track sémantique. Le vrai score CLIP qui a décidé le match sémantique (~0.25-0.28, visible dans les logs `"Semantic candidate scored"`) n'était jamais affiché — le chiffre à l'écran donnait une fausse impression de confiance forte et identique pour les deux conditions, alors que l'une est un match exact (pas de score) et l'autre un match sémantique avec une marge fragile.

**Fix** : le score CLIP qui décide un match sémantique est maintenant threadé jusqu'à l'affichage — `trackedObject.lastScore` (mis à jour dans `matchOrSpawn`/`spawn`, qui prennent désormais un paramètre `score float32`), exposé via `trackedBox.Score` (`boxes()`). `uc_recognition.go` affiche `"<terme> (score 0.XX)"` quand `Score != 0` (terme sémantique), sinon garde `"<terme> (XX.XX%)"` (confiance YOLO, pertinente pour un terme exact). Un terme exact n'a jamais de score CLIP — `Score` reste à 0, comportement d'affichage inchangé pour lui.

Nouveau test (`TestBoxes_ScoreReflectsWhatDecidedTheMatch`) : un terme exact et un terme sémantique sur des boxes différentes, vérifie que seul le second a un `Score` non nul et qu'il correspond exactement au score CLIP mocké.

**Pas re-vérifié visuellement à l'instant du fix** (webcam vide au moment de la capture) — à confirmer par l'utilisateur à son prochain test : le score affiché sur la box sémantique devrait maintenant être ~0.25-0.28 (pas 85%), rendant visible à l'œil que ce match est plus fragile que le match exact.

## 21. Bug plus profond trouvé sous § 20 : `bestMatch` ignorait `filterKey` (2026-08-11, nuit)

L'utilisateur reteste après § 20 et envoie une capture d'écran : les deux pourcentages sont toujours affichés, mais **échangés** — `person (20.95%)` (le terme exact, devrait montrer la confiance YOLO ~85-90%) et `person with a yellow hat (91.06%)` (le terme sémantique, devrait montrer le score CLIP ~0.20-0.28). Aussi demandé : normaliser l'affichage en `%` pour les deux (au lieu de mélanger `"score 0.XX"` et `"XX.XX%"`) — fait en même temps, `tb.Score*100` affiché avec le même format `%.2f%%` que la confiance YOLO.

**Root cause du swap, pas hypothétique** : `bestMatch` (qui décide quel track existant réanchorer sur une box) ne filtrait que par `track.Class` — jamais par `filterKey`. Or un terme exact et un terme sémantique matchés sur le même objet physique (le cas `+overlap`) spawnent deux tracks avec le **même** `track.Class` ("person" pour les deux, hérité du `box.Label` YOLO) mais un `filterKey` différent. Comme `m.active` est une map Go (ordre randomisé), l'appel de la pass 1 (terme exact, `score=0`) pouvait retomber sur le track du terme sémantique plutôt que le sien, lui écrasant `lastScore` à 0 — et symétriquement l'appel de la pass 2 (terme sémantique, `score` réel) pouvait retomber sur le track du terme exact, lui donnant un score qui ne lui appartient pas. Le `filterKey` du track lui-même restait correct (jamais modifié après le spawn) — seul le `lastScore` fuyait vers le mauvais track, d'où l'échange visible à l'écran alors que les identités ("person" vs "person with a yellow hat") restaient bien étiquetées.

**Fix** : `bestMatch` prend maintenant un paramètre `filterKey` et exige `obj.filterKey == filterKey` en plus de `obj.track.Class == box.Label` — un appel d'un terme ne peut plus jamais réanchorer le track d'un autre terme, même si les deux partagent la même classe YOLO.

**Pourquoi les tests précédents ne l'avaient pas attrapé** : tous les tests d'intégration `+overlap` n'appelaient `reanchor()` qu'**une seule fois** — au premier cycle, `m.active` est vide, donc les deux appels passent par `spawn()`, jamais par `bestMatch()` (rien à matcher). Le bug ne se manifeste qu'à partir du **second** cycle, une fois que les deux tracks existent déjà. Nouveau test (`TestReanchor_OverlapTracks_ScoreStaysWithCorrectTrackAcrossCycles`, 5 cycles) — **vérifié qu'il détecte réellement le bug** : fix temporairement retiré, 20/20 runs en échec (la randomisation de la map Go le rend quasi systématique, pas juste occasionnel), fix restauré, 20/20 runs au vert.

**Vérifié visuellement en conditions réelles** (webcam, capture via `/ws`) : `person (89.97%)` et `person with a yellow hat (22.63%)` — les deux formatés en `%`, valeurs enfin correctement associées à leur box (confiance YOLO forte pour l'exact, score CLIP fragile et cohérent avec l'absence réelle de casquette jaune pour le sémantique).

## 22. Confirmation contrôlée : CLIP ne discrimine pas l'attribut "yellow" (2026-08-11, nuit)

Suite de § 14 (hypothèse posée, pas testée) et § 20-21 (bugs d'affichage/association corrigés, le pipeline est maintenant fiable pour ce test). L'utilisateur fait le test contrôlé qui manquait : filtre `person with a yellow hat`, comparé casquette **noire** vs casquette **jaune**, même personne, même cadrage. **Score quasi identique dans les deux cas, la box apparaît à chaque fois.**

**Ça confirme précisément l'hypothèse de § 14, pas juste "CLIP est fragile" en général** : le modèle détecte correctement les concepts "person" et "hat" (la box apparaît de façon cohérente dès qu'il y a une personne avec quelque chose sur la tête), mais **ne lie pas l'attribut couleur "yellow" à l'objet "hat"** — exactement le défaut "bag-of-words cross-modal" documenté dans la littérature (§ 14, CLIP Behaves like a Bag-of-Words Model Cross-modally but not Uni-modally, ICLR 2026). Pas une marge insuffisante (§ 7/§ 10) qu'un seuil mieux calé résoudrait — un défaut structurel de l'alignement cross-modal de CLIP ViT-B/32, documenté comme non résolu dans la recherche actuelle.

**Implication concrète, à trancher** : le filtre texte libre par attribut de couleur (`"person with a yellow hat"`, `"a red car"`, etc.) n'est **pas fiable** avec ce modèle — au mieux équivalent à filtrer sur "person with a hat" (ou même juste "person" selon la marge), pas sur la couleur spécifique. Les filtres texte libre sur un **objet/concept** entier (hors couleur — ex. "a person carrying a box", "an abandoned bag") restent la vraie proposition de valeur de ce mécanisme ; les filtres par **attribut visuel fin** (couleur, motif, petit détail) ne le sont pas avec la configuration actuelle.

**Options pour la suite, aucune tranchée** :
- Accepter la limite, la documenter clairement côté produit/GUI (ne pas promettre le filtrage par couleur) — coût zéro, mais réduit la portée du "langage naturel" annoncé dans la vision du projet.
- Modèle CLIP plus grand (ViT-L/14 ou supérieur) — meilleure résolution spatiale et un peu plus de capacité compositionnelle en général, mais le défaut de binding cross-modal est documenté comme **persistant même sur des modèles plus grands** dans la littérature récente (§ 14) — pas une garantie de fix, coût réel (poids plus lourds, latence plus élevée, § 8).
- Un modèle spécialisé attribut/couleur en aval du crop YOLO (ex. classification de couleur dominante sur la région, indépendant de CLIP) pour les requêtes qui mentionnent explicitement une couleur — changerait l'architecture, pas juste un réglage.
- Ne rien faire de plus pour l'instant, prioriser le reste du backlog (§ H, multi-flux, etc.) — la limite est désormais documentée et confirmée, pas juste supposée.

## 23. Généralisation de § 22 : CLIP discrimine mal la présence même d'un accessoire, pas seulement sa couleur (2026-08-12)

### Schéma du pipeline (discuté avec l'utilisateur, formalisé ici)

```
Boucle vidéo (chaque frame, rapide — jamais YOLO/CLIP)
  frame → tracker KCF/CSRT.Update() par track actif → dessin (boxes()+cascadeOffsets()) → render

Boucle détection (async, ~2-5 fps, reanchor())
  frame → YOLO.AnalyzeFrame()  [toujours, sans filtre — propose TOUTES les boîtes des 80 classes COCO]
        │
        ├─ Passe 1 (exact) : box.Label == term.Key → capture directe, 0 appel CLIP
        │
        ├─ Passe 2 (sémantique) : pour chaque box non capturée par la passe 1 (sauf +overlap),
        │     restreinte par LabelHint si le texte libre mentionne une classe COCO
        │     → crop → CLIP.EncodeImage → cosinus vs CLIP.EncodeText(terme)*
        │     → seuil (0.20) → classement → top-N (cap du terme)
        │
        └─ matchOrSpawn/bestMatch (IoU + filterKey) → cycle de vie du track

  * EncodeText calculé une fois par terme sémantique (newTrackManager), pas par frame.
```

**Conséquence structurelle déjà établie (discussion 2026-08-12, pas testée avant)** : CLIP ne voit jamais une frame entière, seulement un crop qu'YOLO a déjà proposé — si YOLO ne détecte rien sur une frame (candidats vides), la passe 2 n'a rien à scorer, CLIP ne tourne pas ce cycle. Le rappel global du pipeline est donc plafonné par le rappel de YOLO ; CLIP ne peut jamais rattraper un objet que YOLO n'a pas proposé comme candidat, seulement affiner un candidat déjà proposé.

### Constat en direct

Filtre `person*1,person with a hat*1+overlap` (webcam réelle, l'utilisateur seul dans le cadre) : **2 boîtes en continu, avec ou sans casquette**. Contrairement à § 22 (qui isolait l'attribut couleur), ici c'est la présence même de l'accessoire ("a hat") qui ne fait pas varier le résultat.

**Root-cause, avec les chiffres déjà mesurés (§ 7/§ 10)** : `defaultSimilarityThreshold = 0.20`, mais un "person" nu (sans clause) score déjà `~0.235-0.238` en conditions réelles — au-dessus du seuil avant même de considérer la clause ajoutée. Plages mesurées : vrais matches `~0.225-0.29`, confondants `~0.20-0.2476` (chevauchement large, § 10). Le score d'une phrase composée reste dominé par le nom de base ("person", toujours présent puisque `LabelHint` restreint les candidats aux boîtes déjà étiquetées "person" par YOLO — **même boîte que le terme exact**) ; la clause "with a hat" ne fait pas suffisamment redescendre le score en son absence pour repasser sous 0.20. § 14 prédisait exactement ça ("une requête composée a vraisemblablement moins de pouvoir discriminant qu'un nom simple") — confirmé ici pour la présence d'un accessoire, pas seulement sa couleur (§ 22).

**Conséquence pratique** : dans cette configuration, le terme sémantique est quasi redondant avec le terme exact — même boîte, score presque systématiquement au-dessus du seuil, coût CLIP réel payé (~46ms/boîte, § 8) sans gain d'information réel.

### CLIP est-il "overkill" ici ? Sous quelles conditions serait-il réellement utile ?

**Dans cette configuration précise (attribut/accessoire composé avec seuil absolu + LabelHint sur le même objet) : oui, overkill confirmé** — pas seulement pour la couleur (§ 22), généralisé à la présence d'un petit accessoire attaché.

**Pistes d'amélioration, aucune implémentée** :
1. **Scoring différentiel/relatif plutôt qu'un seuil absolu** (piste déjà ouverte en § 7, jamais suivie) : au lieu de `sim(crop, "person with a hat") ≥ 0.20`, comparer contre le nom de base seul — `Δ = sim(crop, "person with a hat") − sim(crop, "person")`, n'accepter que si `Δ` dépasse une marge positive. Isole la contribution marginale de la clause ajoutée plutôt qu'un score absolu dominé par le nom de base — cible directement le mode d'échec observé ici. Coût : un `EncodeText` de plus par terme composé (négligeable, une fois par filtre), aucun `EncodeImage` supplémentaire (déjà calculé). Non implémenté, non testé.
2. **Fine-tuning YOLO (§ A, décision 2026-08-11)** pour les accessoires anticipables (hat, backpack...) — sort ce cas de CLIP entièrement, passe en passe 1 (exact, coût quasi nul).
3. **Histogramme couleur** (déjà décidé) pour l'attribut couleur — sort aussi ce cas de CLIP.
4. **Logique géométrique de tracking** (proximité de boîtes, ex. "sac sans surveillance" — discuté le 2026-08-11) pour les concepts relationnels — pas un problème CLIP du tout.

**Ce qui resterait alors, honnêtement, comme vrai usage de CLIP** :
- Concepts de scène/gestalt non réductibles à objet+attribut+géométrie (ex. "quelque chose de suspect"), **et seulement avec un scoring différentiel/relatif**, jamais validé empiriquement ici — hypothèse, pas un acquis.
- Recherche par **image de référence** (§ D, `EncodeImage` seul, pas de texte composé) — hors du problème de binding texte/image entièrement, régime différent, probablement fiable.
- Filet de repli pour tout concept pas encore fine-tuné (longue traîne), en acceptant une imprécision réelle et documentée, pas silencieuse.

**Bilan honnête (pas d'enjolivement)** : à mesure que couleur (§ 22) et présence d'accessoire (ici) se confirment mal gérées par CLIP tel qu'utilisé actuellement (crop serré + seuil absolu + `LabelHint` sur le même objet que le terme exact), le périmètre où CLIP apporte une vraie valeur ajoutée pour ce produit se réduit à mesure que les cas se testent — pas une remise en cause de CLIP en général, mais de cette configuration précise (seuil absolu, requête composée sur un même objet déjà capturé). Le scoring différentiel (piste 1) est la seule piste non testée qui pourrait changer ce constat sans changer d'architecture ; à date, rien ne prouve qu'il fonctionnerait mieux, juste que c'est la prochaine chose à essayer avant de conclure plus largement.

## 24. Vision cible : CLIP borné à YOLO, décomposition géométrique, grammaire relationnelle extensible (discussion 2026-08-12, rien codé)

Synthèse d'une longue discussion de conception (2026-08-12), consolidée ici avant de se perdre. Rien de cette section n'est implémenté — c'est le cadre cible pour la suite de § A, en remplacement/complément des pistes CLIP pures (scoring différentiel, image↔image, § 23) qui restent possibles mais secondaires désormais.

### CLIP est structurellement dépendant d'une localisation préalable

Établi par élimination successive dans la discussion : CLIP ne dispose d'aucune tête de détection (§ précédent sur "CLIP sans YOLO") — il ne peut **jamais** proposer une région candidate lui-même, seulement juger une région déjà proposée par autre chose (YOLO, une sélection humaine, ou en théorie un autre localisateur). Conséquence stricte : **sans YOLO (ou équivalent) en amont, CLIP n'a rigoureusement rien à faire dans ce pipeline** — y compris pour la galerie de références (§ D) : une sélection humaine unique ne fait que fournir *une cible de comparaison*, elle ne fait pas apparaître de nouveaux candidats sur un autre flux ou aux frames suivantes si la classe concernée n'est pas localisée par ailleurs. Le mode "CLIP sur la frame entière sans YOLO" existe techniquement mais ne produit ni box ni track — dégénéré pour ce produit, pas une vraie alternative.

Implication pour la priorité du backlog : les pistes purement CLIP (scoring différentiel § 23, image↔image) n'ont de valeur que **dans le sous-ensemble déjà localisé par YOLO** — elles ne réduisent jamais la dépendance à YOLO, seulement le taux d'erreur une fois qu'une box existe déjà. Le fine-tuning YOLO (§ A, décision précédente) et la décomposition géométrique ci-dessous n'ont pas cette dépendance : ils étendent ce que YOLO peut localiser lui-même, plus fondamental.

### Décomposition géométrique plutôt que phrase composée CLIP

Pour une requête du type "person with a hat" (ou plus généralement "X avec/portant Y", "X près de Y"), plutôt que de scorer la phrase entière via CLIP (échoue, § 22/23 — le binding attribut/objet et même la présence d'un accessoire composé sont mal discriminés), la décomposer en deux termes **atomiques** (chacun plus fiable seul qu'en phrase composée, § 14/23) reliés par une **relation géométrique** évaluée sur les boîtes déjà localisées :

- **Cas "hat" fine-tuné en classe YOLO** (§ A, décision retenue) : deux boîtes YOLO indépendantes ("person", "hat"), relation vérifiée avec les primitives déjà existantes du domaine (`entities.BoundingBox.IoU`/`Intersection`/`Union`) — **zéro appel CLIP** pour toute la requête composée.
- **Cas "hat" pas encore fine-tuné** : repli sur une sous-région heuristique du conteneur (ex. tiers supérieur de la box "person" pour "hat") + CLIP scoré sur le terme **simple** "hat" (pas la phrase composée) dans cette sous-région — moins précis que le cas fine-tuné, mais strictement meilleur que scorer "person with a hat" sur tout le crop person (déjà testé, § 22/23).

### Vision produit (H2, pas backend) — non retenue comme scope immédiat, notée pour cohérence future

- Champ texte multi-termes avec surlignage live façon regex101, trois familles de termes distinguées visuellement : classe YOLO (exacte/fine-tunée), terme custom (entrée nommée de la galerie de références § D, comparaison image↔image), texte libre CLIP (repli, le moins fiable).
- Par flux, vue graphe interactif des modèles/termes connectés, paramétrable nœud par nœud — matérialise la philosophie ports/adapters déjà en place (chaque terme résolu = un nœud inspectable : lookup YOLO / score CLIP texte / score CLIP image / relation géométrique / futur classifieur dédié). Scope large (type éditeur de nœuds), à séquencer **après** que les briques réelles existent, pas avant (sinon graphe vide).
- Ordre de séquencement proposé : (1) fine-tuning YOLO + relations géométriques (valeur sans dépendance UI) → (2) galerie CLIP promue en termes custom nommables → (3) grammaire relationnelle v2 (ci-dessous) → (4) surlignage live (H2) → (5) éditeur de graphe (H2/H3).

### Grammaire relationnelle — extensible, un seul opérateur tranché pour l'instant

Motif général retenu, pour accueillir de futurs opérateurs paramétrés sans nouvelle refonte de grammaire :
```
term := key ['%' operateur ['=' valeur] '%' key] ['*' cap] ('+' option ['=' valeur])*
```
**Règle de non-ambiguïté (tranchée)** : les suffixes `*cap`/`+option` s'appliquent toujours à la relation **entière** une fois consommée, jamais à l'opérande de droite seul — évite toute ambiguïté positionnelle avec le `+` déjà utilisé pour les options par terme (§ 16), sans avoir besoin de le désambiguïser au cas par cas. Le nom de l'opérateur n'est pas une liste fermée au niveau du tokenizer — seule sa résolution (registre interne nom→fonction d'évaluation géométrique) peut rejeter un nom inconnu, à la manière des options aujourd'hui.

**Un seul opérateur explicitement retenu pour l'instant : `%+%`** (containment — attachement dans une sous-région du conteneur). Les autres (`%near=distance%`, etc.) sont **volontairement différés** — chacun a ses propres paramètres (une distance pour `near`, potentiellement autre chose pour d'autres relations), pas de valeur à les concevoir avant d'avoir un vrai besoin. Ne pas confondre avec `+overlap` (§ 16/17, axe différent — coexistence entre deux **termes de filtre séparés** sur le même objet physique) : la question de cardinalité **à l'intérieur** d'un terme relationnel (ci-dessous) est un axe distinct, volontairement nommé différemment pour ne pas recréer l'ambiguïté déjà rencontrée une fois.

### Décisions de cardinalité (exemple tranché : "personnes près d'un sac", plusieurs sacs, 2026-08-12)

1. **Chaque paire valide (conteneur, attachement) qui satisfait la relation est sa propre instance/match** — pas d'agrégation implicite. 3 sacs proches d'1 personne = 3 paires, pas 1.
2. **`*cap` borne le nombre de paires retenues**, pas le nombre d'instances de chaque côté indépendamment — classées par la métrique propre de la relation quand elle existe (distance croissante pour une future relation `near`), sinon par ordre déterministe (TrackID, même précaution que `cascadeOffsets`, § 19, pour éviter l'ordre randomisé des maps Go).
3. **Appariement 1:1 glouton par défaut** (type assignment biparti) : par cycle, un conteneur et un attachement ne comptent chacun que dans **une seule** paire (la meilleure), cohérent avec le principe déjà acté "pas de chevauchement par défaut" (§ 16, `+overlap`). Une nouvelle option distincte, **`+shared`** (nom volontairement différent de `+overlap` pour ne pas superposer deux sens, cf. ci-dessus), désactivée par défaut, autoriserait un même conteneur/attachement à compter dans plusieurs paires simultanément (N:M) si explicitement demandé.

Rien de tout ça n'est implémenté — grammaire, registre d'opérateurs, `parseFilterSpec`/`reanchor` restent inchangés à ce stade. Prochaine étape suggérée : spécifier `%+%` (containment) seul, bout en bout, avant d'ajouter `%near%` ou `+shared`.

## 25. Scoring différentiel implémenté — le bug "2 boîtes avec ou sans hat" corrigé (2026-08-12)

Suite directe de § 23 : au lieu de continuer la conception de la vision cible (§ 24, rien codé), l'utilisateur a demandé de résoudre d'abord le bug réel déjà diagnostiqué (seuil absolu incapable de discriminer "with a hat" de l'absence de hat, la clause composée ne faisant pas assez redescendre le score sous le seuil).

**Implémentation** : `termMatch.BaseEmbedding` (`internal/application/uc/tracking.go`) — pour tout terme sémantique dont `LabelHint` est non vide, `newTrackManager` encode désormais **aussi** le nom de base seul (ex. "person") en plus du terme composé (ex. "person with a hat"), un appel `EncodeText` de plus, une seule fois par filtre (pas par frame). `reanchor` pass 2 :
```go
accepted := score >= defaultSimilarityThreshold
if term.BaseEmbedding != nil {
    baseScore := cosineSimilarity(embedding, term.BaseEmbedding)
    delta := score - baseScore
    accepted = accepted && delta >= defaultDifferentialMargin
}
```
`defaultDifferentialMargin = 0.02` (nouvelle constante) — **ET** avec le seuil absolu existant, pas un remplacement : un score absolu déjà faible reste rejeté même si l'écart relatif au nom de base paraît important (évite qu'un delta trompeur sur du bruit pur passe le gate). Un terme sans `LabelHint` (concept ouvert, aucun mot COCO mentionné, pas de nom de base à qui se comparer) garde l'ancien comportement au seuil absolu seul — inchangé, pas concerné par ce bug. Log `"Semantic candidate scored"` étendu (`base_score`, `delta`) pour garder la même visibilité de debug que l'existant.

**Mock de test — changement non trivial, à connaître si on retouche ces tests plus tard.** `mockSemanticEncoder.EncodeText` retournait auparavant un vecteur 2D fixe `{1,0}` pour n'importe quel texte — donc, sans changement, le nom de base et le terme composé auraient reçu le **même** embedding, rendant `delta` nul par construction pour **tous** les termes composés existants (quasiment tous les tests, "person with a red/yellow hat" mentionne toujours "person"), rejetant à tort tout ce qui matchait avant. Corrigé en étendant les embeddings à 3D : axe 1 = terme composé (défaut), axe 2 = nom de base (opt-in via le nouveau champ `baseAxisTexts`, requis pour tout terme avec `LabelHint`), axe 3 = composante de normalisation. `scoreByCropSize`/nouveau `baseScoreByCropSize` contrôlent chaque axe indépendamment. **7 tests existants mis à jour** (ajout de `baseAxisTexts: map[string]bool{"person": true}`) — sans ça, `baseScoreByCropSize` non configuré défaut à 0, ce qui à lui seul suffit à préserver la compatibilité (`defaultSimilarityThreshold` 0.20 > `defaultDifferentialMargin` 0.02, donc score ≥ 0.20 implique toujours delta ≥ 0.02 quand la base est à 0) — mais uniquement si le nom de base est bien sur son propre axe, d'où le besoin réel de `baseAxisTexts`.

**Vérification** : 2 nouveaux tests, `TestReanchor_SemanticTerm_DifferentialMarginRejectsBareBaseNoun` (reproduit le bug : score composé 0.27 qui clarifie le seuil absolu, mais score de base 0.26 pour "person" seul — delta 0.01 < 0.02 — doit être rejeté) et `...AcceptsRealSignal` (même score composé 0.27, mais base 0.20 — delta 0.07 — doit matcher normalement, le gate n'est pas un rejet en bloc). **Vérifié que le premier détecte réellement le bug** : condition temporairement neutralisée (`if false && term.BaseEmbedding != nil`), 5/5 runs en échec ; restaurée, tout repasse au vert. `go vet`/`gofmt`/`go test -race` propres sur tout le paquet.

**Confirmé en conditions réelles le 2026-08-12** (webcam, `logs/livesemantic.log`, filtre `person*1,person with a hat*1+overlap`, utilisateur sans hat). Échantillon représentatif (~20 cycles consécutifs, 11:45:36-11:45:41) :
```
score: 0.238, base_score: 0.227, delta: 0.0116  -> above_threshold: false
score: 0.244, base_score: 0.235, delta: 0.0086  -> above_threshold: false
score: 0.241, base_score: 0.227, delta: 0.0142  -> above_threshold: false
score: 0.226, base_score: 0.225, delta: 0.0005  -> above_threshold: false
score: 0.220, base_score: 0.225, delta: -0.0044 -> above_threshold: false
```
`delta` oscille entre -0.005 et +0.014 sur tout l'échantillon, **jamais ≥ `defaultDifferentialMargin` (0.02)** — alors que les scores absolus (0.22-0.24) auraient clairé l'ancien seuil seul (0.20) à chaque fois, exactement le mode d'échec diagnostiqué. `active_tracks` reste à **1** en continu sur toute la fenêtre (`Frame timing`, `kind: reanchor`) — seul le terme exact "person" est actif, la 2e box fantôme a disparu.

**Cas positif confirmé juste après (2026-08-12, 11:52, même session), utilisateur avec un vrai hat cette fois** :
```
score: 0.238, base_score: 0.216, delta: 0.0206 -> above_threshold: true
score: 0.230, base_score: 0.209, delta: 0.0212 -> above_threshold: true
score: 0.252, base_score: 0.214, delta: 0.0382 -> above_threshold: true
score: 0.244, base_score: 0.216, delta: 0.0284 -> above_threshold true
```
`active_tracks: 2` sur la majorité des cycles de cette fenêtre — les deux boîtes ("person" exact + "person with a hat" sémantique) coexistent bien, `+overlap` fonctionne toujours en combinaison avec le nouveau gate. Le mécanisme fait donc bien la différence dans les deux sens (sans hat → rejeté, avec hat → accepté) sur ce test.

**Point à noter, pas un bug** : le `delta` avec hat oscille dans une fourchette assez large (0.004 à 0.038 selon les cycles, cf. logs 11:52:45-47) — plusieurs cycles individuels retombent sous `0.02` même avec le hat porté (angle de vue, éclairage, mouvement), donnant `above_threshold: false` ponctuellement. `active_tracks` reste néanmoins à 2 en continu grâce à la tolérance de la machine à états du track (`StateCoasting`, `maxMissesBeforeLost=5`) — un miss ponctuel ne fait pas disparaître le track. Ça confirme que `0.02` est une marge **serrée** par rapport au signal réel mesuré ici (souvent proche de la limite plutôt que largement au-dessus) — fonctionne, mais pas avec une confortable, à garder en tête si un signal plus faible (angle différent, hat moins net) se présente. Pas recalibré pour l'instant, comportement jugé correct par l'utilisateur en l'état.

## 26. `%+%` (containment géométrique) implémenté (2026-08-12, branche `feat/relational-operator`)

Première réalisation concrète de la vision § 24 : décomposer "conteneur avec/portant attachement" en deux détections YOLO indépendantes reliées par une relation géométrique, plutôt qu'une phrase composée scorée par CLIP.

**Grammaire** (`filter_spec.go`) : `container%relation[=param]%attachment`. Point technique notable — `splitRelation` doit tourner **avant** tout split sur `+`, parce que l'unique opérateur retenu s'appelle littéralement `+` (`%+%`) : découper d'abord sur `+` (comme le fait déjà le parsing des options `+overlap`) fragmenterait "%+%" lui-même. `relationOperators` est un registre fermé (un seul nom aujourd'hui : `+`) — un nom inconnu erreure clairement, cohérent avec la philosophie déjà en place pour les options.

**v1 volontairement restreinte** : conteneur et attachement doivent tous les deux être des classes COCO exactes — pas de CLIP, pas de repli sur une sous-région heuristique pour l'instant (ces pistes restent en § 24, pour plus tard).

**Matching** (`tracking.go`, nouvelle Pass 0 dans `reanchor`, avant les passes exact/sémantique existantes) : pour chaque terme relationnel, apparie les boîtes conteneur/attachement via `containmentRatio` — la fraction de la boîte **attachement** couverte par le conteneur, pas un IoU classique (un petit sac dans une grande personne aurait un IoU faible par construction — l'union est dominée par la personne — malgré un vrai confinement total ; `containmentRatio` capture précisément ce cas). Seuil `relationContainmentThreshold = 0.5`, valeur de départ non calibrée. Cardinalité conforme à § 24 : chaque paire valide est une instance, appariement glouton 1:1 par défaut (une boîte ne rejoint qu'une seule paire par cycle), classé par ratio décroissant, top-`Cap` gardées. La boîte **conteneur** devient l'entité suivie/dessinée (comme un terme exact) ; l'attachement sert uniquement à la décision, pas suivi séparément — scope MVP, une extension possible mais non retenue ici.

**Tests** : 8 nouveaux (grammaire + matching), y compris un test d'exclusivité gloutonne vérifié par désinactivation temporaire de la garde correspondante (échec reproductible sans elle, vert avec). `go vet`/`gofmt`/`go test -race` propres.

**Testé en conditions réelles le 2026-08-12** — trois étapes de diagnostic en direct :

1. **Premier essai `person%+%backpack` : aucune box.** Diagnostiqué grâce au logging ajouté juste après coup (branche `fix/relational-pass-visibility`, mergée séparément) : `"Relational term has no candidates this cycle"`, `attachment_candidates: 0` en continu sur des dizaines de cycles — YOLO ne détectait jamais "backpack" du tout, indépendamment de la logique de containment.
2. **Isolé avec un filtre `backpack` seul** : `active_tracks` clignote entre `0` et `1` — détection intermittente/instable confirmée (pas "jamais détecté", juste peu fiable à cet angle/cette distance).
3. **Retest de `person%+%backpack`, pose plus stable — plusieurs vrais matchs** :
```
ratio: 1.0       -> above_threshold: true
ratio: 1.0       -> above_threshold: true
ratio: 0.976     -> above_threshold: true
ratio: 0.812     -> above_threshold: true
```
La logique de containment est donc validée avec de vrais chiffres, pas seulement en tests unitaires synthétiques.

**Pourquoi l'utilisateur n'a rien vu à l'écran malgré ces matchs confirmés en logs** : `maxMissesBeforeLost = 2` (`entities/track.go`, réglé lors d'une investigation perf antérieure, § F — sans rapport avec ce chantier) tue un track après 2 ratés **consécutifs**. Avec une détection "backpack" aussi instable (match, puis 2-4 cycles sans candidat, répété), le track créé à chaque match meurt quasiment aussitôt — durée de vie réelle de l'ordre d'un cycle (~200ms), largement sous le seuil de perception visuelle. **Pas un bug de `%+%`** — la même limitation de stabilité de détection YOLO déjà documentée (§ 9/§ 10), simplement rendue visible différemment ici. Non retouché : `maxMissesBeforeLost` est une constante globale, la changer affecterait tous les tracks (exact/sémantique/relationnel), pas seulement ce cas d'usage précis — pas une décision à prendre à la légère sans plus de recul.

## 27. `%near=distance%` (proximité géométrique) implémenté (2026-08-12, branche `feat/near-relation-operator`)

Deuxième opérateur relationnel, suite directe de § 26 — même grammaire (`container%relation[=param]%attachment`), nouveau registre `relationOperatorSpec{requiresParam bool}` : `+` n'accepte aucun paramètre (erreur si fourni), `near` en exige un (erreur si absent, non numérique, ou ≤ 0) — validé une fois à `parseFilterSpec`, stocké déjà parsé en `float32` (`filterTerm.RelationParam`/`termMatch.RelationParam`, plus une chaîne à reparser plus tard).

**Métrique retenue pour "near" : écart de bord à bord (`boxGap`), pas centre à centre.** Une personne et une voiture garée juste à côté peuvent avoir des centres éloignés (grandes boîtes) tout en étant adjacentes — une distance centre-à-centre pénaliserait injustement les grandes boîtes. `boxGap` calcule le vide entre les rectangles sur chaque axe (0 si superposition/contact), distance euclidienne des deux écarts.

**Pass 0 généralisée** : le calcul de métrique et le tri dépendent maintenant de `term.Relation` (switch à deux branches — `containmentRatio`/décroissant pour `+`, `boxGap`/croissant pour `near`, le plus proche gagne) plutôt qu'un seul chemin ratio-only. Cardinalité/appariement glouton 1:1 inchangés (§ 24/26).

**Tests** : 3 nouveaux tests `reanchor` (match sous le seuil, rejet au-dessus, classement — ce dernier construit délibérément deux paires indépendantes puisque seule la boîte **conteneur** est suivie, une paire seule ne peut pas démontrer un classement) + 5 nouveaux cas de grammaire (paramètre requis/rejeté selon l'opérateur, non-numérique, non-positif). **Classement vérifié détecter une régression réelle** : tri désactivé temporairement (`if false && term.Relation == "near"`), 5/5 échecs reproductibles ; restauré, tout vert.

**Testé en conditions réelles** : câblage confirmé réactif (`person%near=500%chair` sur webcam locale, logs `"Relational term has no candidates this cycle"` à chaque cycle) — pas de scène avec une vraie correspondance disponible pendant ce test automatisé (personne physiquement devant la caméra), donc le cas positif reste à valider visuellement par l'utilisateur. `go vet`/`gofmt`/`go test -race` propres.

## 29. `+shared` (cardinalité N:M) implémenté (2026-08-12, branche `feat/shared-relation-option`)

Dernière pièce de la grammaire relationnelle actée en § 24 : par défaut l'appariement conteneur/attachement reste glouton 1:1 (une boîte ne rejoint qu'une seule paire par cycle, § 26) — `+shared` (même grammaire que `+overlap` : `+shared` seul = `true`, `+shared=false/true` explicite) lève cette exclusivité, autorisant une même boîte à satisfaire plusieurs paires. Nom délibérément distinct de `+overlap` (rappel § 16/24) : `+overlap` gère la coexistence **entre deux termes de filtre séparés** sur un même objet physique, `+shared` gère la cardinalité **à l'intérieur d'un seul terme relationnel** — deux axes différents, pas à fusionner sous un même nom.

**Validation** : `+shared` n'a de sens que sur un terme relationnel (`Relation != ""`) — rejeté avec une erreur claire sur un terme exact/sémantique (`person+shared`), plutôt qu'un no-op silencieux.

**Implémentation** : la boucle de sélection de `reanchor` (Pass 0) ne consulte plus `usedContainer`/`usedAttachment` que si `!term.Shared` — reste renseignées dans les deux cas (bookkeeping neutre quand `Shared`, exclusion réelle sinon).

**Piège de test identifié en écrivant les tests** : comme seule la boîte **conteneur** est effectivement suivie (§ 26, scope MVP), un test avec un conteneur partagé entre plusieurs attachements ne produit **aucune différence observable** (même track re-matché plusieurs fois, idempotent) — `+shared` n'a d'effet visible que quand c'est l'**attachement** qui est partagé entre plusieurs conteneurs différents (ex. un sac au sol près de deux personnes différentes → deux tracks "person" distincts avec `+shared`, un seul sans). Les tests utilisent ce cas précis (`TestReanchor_RelationalTerm_SharedAllowsSameAttachmentInMultiplePairs`) plutôt que l'inverse.

**Tests** : 5 cas de grammaire (bool implicite/explicite, combiné à `*cap`+`+overlap`, rejeté sur terme non-relationnel, valeur non-bool) + 2 tests `reanchor` (avec/sans `+shared` sur la même scène — sac partagé entre deux personnes qui se chevauchent). **Vérifié détecter une régression réelle** : garde `!term.Shared` retirée temporairement, 3/3 échecs reproductibles sur le test positif (le test négatif restait vert, cohérent) ; restaurée, tout vert. Câblage confirmé réactif en conditions réelles (webcam locale, `person%+%backpack*2+shared`), pas de scène disponible pour un vrai match positif pendant ce test automatisé. `go vet`/`gofmt`/`go test -race` propres.

**Grammaire relationnelle du § 24 désormais complète** : `%+%`, `%near=distance%`, `+shared` tous implémentés et testés. Reste ouvert (hors scope, pas commencé) : promotion de la galerie CLIP en termes nommables référençables dans le filtre, surlignage live type regex101, graphe interactif — tout ça reste H2/produit, séquencé après le backend.

## 30. Galerie CLIP promue en termes nommables (2026-08-12, branche `feat/reference-gallery`)

Troisième famille de terme de filtre, aux côtés des classes COCO exactes et du texte libre CLIP — TODO.md § D "reconnaissance par référence image" / § H1 "Galerie de références", promue en § 24 : une entrée de galerie nommée devient directement utilisable comme clé de terme dans le filtre.

**Store** (`internal/application/uc/gallery.go`, `ReferenceGallery`) : map `{nom, embedding, enabled}` en mémoire, protégée par un `sync.RWMutex` (pas de base vectorielle à cette échelle, cf. § D d'origine). `Add` rejette un nom vide, un embedding vide, un doublon, **et une collision avec une classe COCO** — un nom de galerie identique à une classe COCO ne serait jamais atteignable (`newTrackManager` vérifie COCO en premier, sans condition) : mieux vaut une erreur claire à l'ajout qu'une entrée silencieusement inutilisable. `Rename`/`SetEnabled`/`Remove`/`List` complètent le CRUD ; `Get` traite une entrée désactivée comme absente (pas un "match vide" silencieux) et est sûre sur un récepteur `nil` (défense en profondeur, `uc.gallery` toujours initialisé par `NewUseCase` mais coût nul à vérifier quand même).

**Intégration `newTrackManager`** : pour un terme non-COCO, la galerie est consultée **avant** l'appel `EncodeText` — un nom enregistré ne paie/ne risque jamais un encodage texte. Aucun `LabelHint`/`BaseEmbedding` pour un terme de galerie (pas de texte libre à scanner pour un mot COCO, pas de "nom de base" contre lequel calculer un delta, § 23) — retombe sur le seuil absolu seul, comme tout terme sans `LabelHint`. La passe 2 de `reanchor` elle-même est **inchangée** : elle calcule une similarité cosinus entre l'embedding du candidat et `term.Embedding`, sans se soucier de son origine (texte ou image) — c'est cette indifférence qui rend l'intégration presque gratuite.

**REST** (`internal/transport/adapters/api/gallery.go`) : `POST /api/v1/gallery` (multipart, champs `name`+`image`, décodage JPEG/PNG), `GET /api/v1/gallery`, `PATCH /api/v1/gallery/:name` (renommage et/ou activation en un seul appel), `DELETE /api/v1/gallery/:name` (idempotent).

**Tests** : 15 cas sur `ReferenceGallery` (CRUD, collisions, normalisation, réception `nil`), 3 sur les méthodes `UseCase` (encodage réel via le mock, propagation d'erreur), 2 d'intégration `newTrackManager`/`reanchor` (résolution galerie sans `EncodeText`, match réel par similarité image↔image), 9 sur les handlers REST (upload multipart réel avec une vraie image PNG encodée à la volée, erreurs 400 propagées, idempotence du delete).

**Testé en conditions réelles, bout en bout, avec du vrai CLIP** (pas des mocks) : upload d'une frame JPEG réelle (extraite de `assets/videos/person.mp4`) via `curl -F`, `EncodeImage` réel, stockage, listage, renommage, activation/désactivation, suppression — tous vérifiés via l'API. Puis filtre `mon_ref` (le nom de la galerie) démarré en session réelle : logs `"Semantic candidate scored"` confirmant un vrai scoring image↔image contre les candidats YOLO de la webcam (scores 0.48-0.61, tous au-dessus du seuil). **Observation notée, pas un bug** : le meilleur score n'était pas contre "person" (la classe de l'image de référence) mais contre "potted plant"/"chair" — cohérent avec la fragilité déjà documentée (§ 7/10/22/23), maintenant observée sur le chemin image↔image plutôt que texte↔image ; pas creusé plus loin ici, à garder en tête si la galerie devient un chantier prioritaire. `go vet`/`gofmt`/`go test -race` propres sur tout le repo.

**Pas fait** : l'UI de sélection en direct (clic sur une box → nom → ajout, `docs/gui/spec.md` § 3.2) reste H2, pas commencée — cette passe couvre uniquement le backend + l'API REST.

## 31. Pipeline de fine-tuning YOLO — scaffolding, PAS exécuté (2026-08-12, branche `feat/yolo-finetune-pipeline`)

**Niveau de confiance différent de tout ce qui précède dans ce document** : ce qui suit n'a **pas** été testé en conditions réelles — contrairement à chaque autre section de cette ADR (webcam réelle, logs réels, revert/restore vérifié). Diagnostic matériel de cet environnement (2026-08-12) : Intel i7 6 cœurs + AMD Radeon Pro 560X (pas de GPU CUDA — le backend MPS de PyTorch ne supporte que Apple Silicon, pas cet AMD discret), 16 Go RAM, **~8 Go de disque libre seulement**. Un vrai entraînement (même limité à une classe) demande réalistement un GPU CUDA et des dizaines de Go de disque — ni l'un ni l'autre n'est disponible ici. Plutôt que de simuler un entraînement ou de prétendre l'avoir fait, le choix a été de construire le **pipeline réutilisable** (TODO.md § A le demandait explicitement : "versionné dans le repo") sans l'exécuter — à lancer sur une machine qui a le matériel.

**`training/`** (nouveau dossier, hors du module Go — Python) :
- `prepare_dataset.py` — télécharge un sous-ensemble de la classe "Hat" d'Open Images V7 via FiftyOne (déjà annotée avec des boîtes, pas de labellisation manuelle), export au format YOLO. Volontairement petit par défaut (quelques centaines d'images) pour rester traitable sur un ordinateur portable, pas dimensionné pour de la précision réelle.
- `train.py` — fine-tune depuis le checkpoint `yolo11s.pt` COCO pré-entraîné d'Ultralytics, étend la liste de classes de 80 à 81. **Ordre des 80 classes COCO vérifié caractère pour caractère identique à `entities.Yolo11sClasses()`** (script de comparaison Python ad hoc, pas juste relu à l'œil) — un décalage d'index ici casserait silencieusement toute la classification côté Go.
- `README.md` — prérequis, étapes, et surtout un vrai avertissement en tête de fichier ("Reality check").

**Défaut connu, documenté dans `train.py` lui-même, pas contourné** : `prepare_dataset.py` ne télécharge que des images labellisées "Hat" — aucune des 80 classes COCO d'origine n'a ses propres boîtes dans ces images (les annotations Open Images pour les objets co-présents n'ont pas été récupérées). Entraîner une tête à 81 classes uniquement sur ces images risque un vrai **oubli catastrophique** des 80 classes existantes (le modèle voit des images où une personne/voiture est peut-être visible mais jamais étiquetée, et peut apprendre à ne plus les détecter). Mitigé (pas éliminé) par `freeze=10` (gèle la majorité du backbone), un taux d'apprentissage bas et peu d'époques — pratique standard pour l'ajout étroit d'une seule classe, mais pas équivalent à la vraie solution (mélanger un sous-ensemble réel de COCO avec ses annotations d'origine, non fait ici faute de place disque).

**Ce qui reste à faire, sur une machine avec le matériel** : exécuter `prepare_dataset.py` puis `train.py`, exporter en ONNX (opset 19 pour rester compatible avec le runtime actuel — `yolo11s.go` documente déjà cette contrainte), évaluer sur un sous-ensemble de validation COCO avant de faire confiance au résultat au-delà de "hat", puis mettre à jour manuellement `entities/class.go`/`yolo11s.go` côté Go (`modelClasses = 81`) et remplacer `assets/models/yolo11s.onnx` — étape volontairement manuelle/relue, pas automatisée, vu l'importance de ce modèle dans tout le pipeline.

## 32. Protocole `/ws` enrichi — boîtes en JSON séparé (2026-08-12, branche `feat/ws-structured-boxes`)

TODO.md § H2, préparatoire à l'intégration des maquettes GUI (`docs/gui/mockups/`, écrans 1c/1d/1h — boxes dessinées comme éléments DOM cliquables/survolables, pas des pixels figés). Écart assumé depuis § 26 ("boxes déjà dessinées par `RecognitionUseCase`, pas de JSON séparé — suffisant pour valider le transport") — maintenant comblé.

**Nouveau port** (`internal/infrastructure/streamer/streamer.go`) : `BoxData` (label déjà formaté, `TrackID`, coordonnées **normalisées [0,1]** — un client n'a pas besoin de connaître la résolution source pour positionner un overlay) et `BoxAwareOutputStream` (capacité optionnelle, vérifiée par assertion de type dans `uc_recognition.go`, pas ajoutée à l'interface `OutputStream` de base — la fenêtre gocv/CLI n'a pas de canal séparé pour du structuré et n'en a pas besoin).

**`WebSocketOutput`** implémente désormais les deux : `Render` (JPEG binaire, image **non annotée** quand un client box-aware est branché) et `RenderBoxes` (JSON texte, enveloppe `{"boxes":[...]}` — un wrapper plutôt qu'un tableau nu, pour pouvoir ajouter un futur second type de message sans ambiguïté côté client). Toujours envoyé, même vide (`{"boxes":[]}`) — un client doit pouvoir distinguer "rien détecté ce cycle" d'une connexion qui traîne.

**`uc_recognition.go`** : la logique de formatage du label (`boxDescription`, extraite, partagée entre les deux chemins) tourne à l'identique quel que soit le mode de sortie. `cascadeOffsets` (décalage pixel des boxes quasi-identiques, § 19) reste réservé au dessin serveur — un client box-aware peut décaler ses labels avec un vrai layout DOM, plus besoin du contournement pixel.

**Tests** : 3 nouveaux sur `WebSocketOutput` (diffusion JSON à plusieurs clients, message vide explicite, client mort abandonné — même sémantique que `Render`).

**Testé en conditions réelles** : client WS Python maison distinguant les frames binaires (JPEG, SOI `0xFFD8` confirmé, non annoté) des frames texte (`{"boxes":[]}`, envoyées à chaque cycle même sans détection) sur la même connexion `/ws`, session réelle démarrée/arrêtée proprement (pas de SIGABRT/SIGSEGV). `go vet`/`gofmt`/`go test -race` propres sur tout le repo.

**Pas fait** : le frontend (`web/`) ne consomme pas encore ce nouveau canal — `VideoStream.tsx` affiche toujours l'image telle quelle, sans dessiner d'overlay. Prochaine étape logique côté H2.

## 33. Références

- Modèle original : [openai/clip-vit-base-patch32](https://huggingface.co/openai/clip-vit-base-patch32) (licence MIT, [openai/CLIP](https://github.com/openai/CLIP))
- Export ONNX retenu : [Xenova/clip-vit-base-patch32](https://huggingface.co/Xenova/clip-vit-base-patch32)
- Tokenizer de référence : `openai/CLIP` → `clip/simple_tokenizer.py` (BPE, vocab 49408 tokens)
