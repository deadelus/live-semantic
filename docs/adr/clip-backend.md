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

## 7. Références

- Modèle original : [openai/clip-vit-base-patch32](https://huggingface.co/openai/clip-vit-base-patch32) (licence MIT, [openai/CLIP](https://github.com/openai/CLIP))
- Export ONNX retenu : [Xenova/clip-vit-base-patch32](https://huggingface.co/Xenova/clip-vit-base-patch32)
- Tokenizer de référence : `openai/CLIP` → `clip/simple_tokenizer.py` (BPE, vocab 49408 tokens)
