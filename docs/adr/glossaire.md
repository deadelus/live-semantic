# Glossaire technique — pipeline LiveSemantic

> Vocabulaire du pipeline complet : vidéo → préprocessing → inférence → sémantique → serving.
> Chaque entrée est autonome. Les renvois entre termes sont notés → *terme*.

---

## 1. Graphe & formats de modèle

**Tenseur**
Tableau multidimensionnel typé — l'unité de donnée qui circule dans un modèle. Une image RGB 224×224 en batch de 1 est un tenseur de shape `[1, 3, 224, 224]`.

**Shape**
Les dimensions d'un tenseur. `[1, 3, 224, 224]` = batch 1, 3 canaux, 224 de haut, 224 de large.

**Rang (rank)**
Le nombre de dimensions. Un scalaire est de rang 0, un vecteur de rang 1, une image batchée de rang 4.

**dtype**
Le type numérique des éléments : `float32`, `float16`, `int8`, `bool`… Détermine la précision et l'empreinte mémoire.

**Layout (NCHW / NHWC)**
L'ordre des dimensions. **NCHW** (batch, canaux, hauteur, largeur) est la convention PyTorch/ONNX. **NHWC** est celle de TensorFlow et de beaucoup d'accélérateurs. Une conversion mal placée coûte une transposition mémoire à chaque frame.

**Shape dynamique**
Dimension laissée variable à l'export (souvent le batch, parfois la résolution), notée par un nom symbolique au lieu d'un entier. Plus flexible, mais bloque certaines optimisations du runtime qui ont besoin de tailles connues.

**Graphe de calcul**
Représentation du modèle comme un DAG : les nœuds sont des opérations, les arêtes des tenseurs.

**Nœud / opérateur (op)**
Une opération élémentaire du graphe : `Conv`, `MatMul`, `Softmax`, `Resize`, `Gather`.

**Opset (operator set)**
Numéro de version du catalogue d'opérateurs ONNX. Fige le comportement de chaque op. → voir la note dédiée dans `inference-runtimes.md`.

**Domain**
Espace de nommage des opérateurs. Le domaine vide `""` est le standard ONNX ; `com.microsoft` regroupe les extensions ORT.

**Initializer**
Les poids entraînés, stockés en dur dans le fichier `.onnx` comme tenseurs constants.

**Protobuf**
Le format de sérialisation binaire de Google utilisé par ONNX. Explique pourquoi un `.onnx` n'est pas lisible en texte.

**IR (Intermediate Representation)**
Représentation pivot entre le framework d'entraînement et le compilateur cible. ONNX et StableHLO en sont deux.

**StableHLO**
L'IR portable de l'écosystème Google/JAX. Joue le rôle qu'ONNX joue côté PyTorch.

**Custom op**
Opérateur non standard, défini par l'utilisateur. Casse la portabilité : le runtime cible doit fournir une implémentation.

---

## 2. Export

**Tracing**
Méthode d'export historique : on exécute le modèle une fois avec un tenseur d'exemple et on enregistre les opérations traversées. Problème — tout contrôle de flux dépendant des données est « gelé » dans la branche prise pendant la trace.

**Scripting (TorchScript)**
Analyse statique du code Python pour capturer les branches et boucles. Plus fidèle que le tracing, mais supporte un sous-ensemble restreint de Python. Largement remplacé par dynamo.

**Dynamo / `torch.export`**
Le chemin d'export moderne de PyTorch. Capture le graphe au niveau bytecode, gère mieux le contrôle de flux et les shapes dynamiques. C'est le mode à utiliser aujourd'hui.

**FX graph**
La représentation intermédiaire de PyTorch produite par dynamo, avant conversion vers ONNX.

**Shape inference**
Passe qui propage les shapes à travers le graphe pour les déduire à chaque nœud. Un échec de shape inference est souvent le premier symptôme d'un export mal formé.

**Constant folding**
Optimisation qui pré-calcule les sous-graphes dont toutes les entrées sont constantes, et remplace le résultat par une constante.

**onnx-simplifier**
Outil qui nettoie un graphe exporté : constant folding, suppression des nœuds morts, fusion de motifs redondants. Passage quasi systématique après export.

---

## 3. Runtime & exécution

**Runtime d'inférence**
Le moteur qui charge un modèle et l'exécute. ONNX Runtime, TensorRT, OpenVINO, LiteRT.

**Session (`InferenceSession`)**
L'objet ORT qui encapsule un modèle chargé et optimisé. Sa création est coûteuse — on la fait une fois au démarrage, jamais par frame.

**Execution Provider (EP)**
Le backend auquel ORT délègue le calcul : `CPUExecutionProvider`, `CUDAExecutionProvider`, `TensorrtExecutionProvider`, `CoreMLExecutionProvider`… On en fournit une liste ordonnée par préférence.

**Node placement / partitioning**
La phase où ORT décide quel EP exécute quel nœud. Le graphe est découpé en sous-graphes assignés à des backends différents.

**Fallback CPU**
Quand un opérateur n'est pas supporté par l'EP prioritaire, ORT le place sur CPU. Silencieux, et générateur d'allers-retours mémoire GPU↔CPU qui détruisent la latence. → à traquer par profiling.

**Kernel**
L'implémentation concrète d'un opérateur pour un matériel donné. Un même `Conv` a un kernel CPU AVX-512, un kernel CUDA, un kernel Metal.

**Fusion d'opérateurs**
Optimisation qui fond plusieurs nœuds en un seul kernel (`Conv` + `BatchNorm` + `ReLU` → un kernel unique), évitant des écritures mémoire intermédiaires.

**Graph optimization level**
Réglage ORT : `disabled`, `basic`, `extended`, `all`. Contrôle l'agressivité des passes d'optimisation au chargement.

**IO Binding**
Mécanisme ORT permettant de pré-allouer les tenseurs d'entrée/sortie directement sur le device cible, pour éviter une copie à chaque appel. Essentiel en temps réel sur GPU.

**Memory arena**
Allocateur qui réserve un gros bloc mémoire au démarrage et le recycle entre inférences, plutôt que de faire des malloc/free par appel.

**Intra-op / inter-op parallelism**
Deux niveaux de parallélisme CPU. *Intra-op* = paralléliser à l'intérieur d'un opérateur (les threads d'un même `MatMul`). *Inter-op* = exécuter plusieurs nœuds indépendants en parallèle. Mal réglés, ils se battent pour les cœurs.

**Warm-up**
Quelques inférences à vide au démarrage pour déclencher les allocations paresseuses, la compilation JIT des kernels et le remplissage des caches. Sans ça, la première mesure de latence est trompeuse.

---

## 4. Compilation

**AOT / JIT**
*Ahead-Of-Time* : le modèle est compilé une fois, avant l'exécution (TensorRT). *Just-In-Time* : compilé au premier appel (XLA). L'AOT donne une latence stable dès la première frame, le JIT une phase de chauffe.

**XLA**
Le compilateur d'algèbre linéaire de Google, moteur de JAX et TensorFlow.

**TensorRT engine (plan file)**
Le binaire produit par TensorRT après compilation. **Lié au GPU et à la version de TensorRT** — il faut le régénérer à chaque changement de machine cible. À ne pas versionner dans le repo.

**Autotuning**
Recherche empirique de la meilleure implémentation d'un opérateur pour un matériel donné : le compilateur benchmarke plusieurs variantes et garde la plus rapide. Cœur de TVM et IREE.

**TVM / IREE**
Compilateurs ML qui génèrent du code natif optimisé par autotuning. Mise en œuvre lourde, gains importants sur du hardware non standard.

---

## 5. Précision & quantification

**FP32 / FP16 / BF16**
Flottants sur 32 et 16 bits. **BF16** garde la plage d'exposant du FP32 avec moins de mantisse — plus stable que le FP16 pour l'entraînement, largement supporté en inférence moderne.

**INT8 / INT4**
Entiers 8 et 4 bits. Réduisent la mémoire d'un facteur 4 à 8 et exploitent des unités matérielles dédiées, au prix d'une perte de précision à mesurer.

**Quantification**
Conversion des poids et/ou activations vers une précision réduite.

**Scale / zero-point**
Les deux paramètres de la conversion : `réel ≈ scale × (entier − zero_point)`.

**Per-tensor / per-channel**
Granularité de la quantification. *Per-tensor* = un seul couple scale/zero-point pour tout le tenseur. *Per-channel* = un par canal de sortie — plus précis, standard pour les convolutions.

**PTQ (Post-Training Quantization)**
Quantifier un modèle déjà entraîné. Rapide, sans réentraînement.

**QAT (Quantization-Aware Training)**
Simuler la quantification pendant l'entraînement pour que le modèle s'y adapte. Meilleure précision finale, mais nécessite le pipeline d'entraînement.

**Calibration**
Passage d'un échantillon représentatif de données dans le modèle pour mesurer la plage des activations et en déduire les scales. **La qualité du jeu de calibration détermine la qualité du modèle quantifié.**

**QDQ (QuantizeLinear / DequantizeLinear)**
La paire de nœuds ONNX qui encode la quantification dans le graphe. Le runtime les reconnaît et les fusionne dans les kernels entiers.

**Pruning**
Suppression de poids ou de structures jugés peu utiles pour alléger le modèle.

**Distillation**
Entraîner un petit modèle à imiter les sorties d'un gros. Réduction de taille sans changer l'architecture d'inférence.

---

## 6. Performance

**Latence**
Temps pour traiter **une** requête, de l'entrée à la sortie. C'est la métrique qui compte en temps réel.

**Débit (throughput)**
Nombre de requêtes traitées par seconde. Optimiser le débit dégrade souvent la latence, et inversement.

**p50 / p95 / p99**
Percentiles de latence. La p50 est la médiane, la p99 le pire cas sur 1 % des requêtes. **Toujours rapporter au moins la p95** — une moyenne cache les à-coups.

**Batching**
Grouper plusieurs entrées en un seul appel pour amortir le coût fixe. Améliore le débit, ajoute de la latence d'attente.

**Batching dynamique**
Le serveur regroupe automatiquement les requêtes arrivant dans une petite fenêtre temporelle. Fonctionnalité clé de Triton.

**Compute-bound / memory-bound**
Un kernel est *compute-bound* si sa vitesse est limitée par les unités de calcul, *memory-bound* si elle l'est par la bande passante mémoire. Le diagnostic dicte la stratégie d'optimisation — quantifier aide surtout dans le second cas.

**H2D / D2H**
*Host-to-Device* et *Device-to-Host* : les transferts CPU↔GPU sur PCIe. Souvent le vrai goulot en vision temps réel, bien avant le calcul lui-même.

**Zero-copy**
Éviter la duplication d'un buffer en mémoire en passant un pointeur plutôt qu'une copie. Décisif entre le décodeur vidéo et le préprocessing.

---

## 7. Serving & déploiement

**Triton Inference Server**
Serveur NVIDIA multi-framework : sert ONNX, TensorRT, PyTorch, TF simultanément, avec batching dynamique et gestion de versions. ⚠️ Sans rapport avec le **langage Triton d'OpenAI**, qui sert à écrire des kernels GPU.

**Model repository**
L'arborescence de fichiers que Triton surveille : un dossier par modèle, un sous-dossier par version, plus un `config.pbtxt`.

**Instance group**
Configuration Triton indiquant combien de copies d'un modèle charger et sur quels devices. Permet d'exploiter plusieurs GPU ou de saturer un seul.

**Ensemble**
Pipeline déclaratif dans Triton chaînant plusieurs modèles (préprocessing → encodeur → postprocessing) en un seul appel client.

**TF Serving / TorchServe**
Les équivalents mono-framework, respectivement pour TensorFlow et PyTorch.

**KServe / BentoML / Ray Serve**
Couche au-dessus du serveur : packaging, autoscaling, routage, déploiement Kubernetes.

---

## 8. Vidéo & acquisition

**Frame**
Une image du flux. L'unité de traitement du pipeline.

**FPS**
Images par seconde. À distinguer : le FPS **source** (ce que produit la caméra) et le FPS **traité** (ce que le pipeline arrive à absorber).

**Codec / conteneur**
Le *codec* est l'algorithme de compression (H.264, H.265, AV1). Le *conteneur* est le format de fichier qui emballe les flux (MP4, MKV, AVI). Un `.mp4` ne dit rien du codec à l'intérieur.

**I-frame / P-frame / B-frame**
Types d'images compressées. Une **I-frame** (keyframe) est autonome. Les **P** et **B** ne codent que les différences avec d'autres frames. Conséquence pratique : on ne peut pas se positionner arbitrairement dans un flux, seulement sur une keyframe.

**GOP (Group of Pictures)**
L'intervalle entre deux keyframes. Détermine la granularité du seek et la latence de reprise sur un flux réseau.

**Décodage hardware (NVDEC / VAAPI / VideoToolbox)**
Décompression déléguée à un circuit dédié du GPU au lieu du CPU. Indispensable pour du multi-flux temps réel — et permet de garder la frame en mémoire GPU sans repasser par le CPU.

**RTSP / RTMP / HLS / WebRTC**
Protocoles de streaming. **RTSP** domine sur les caméras IP. **HLS** est segmenté et introduit plusieurs secondes de latence. **WebRTC** est le plus faible latence.

**Backpressure**
Ce qui se passe quand le pipeline est plus lent que la source. Deux stratégies : bufferiser (la latence croît sans limite) ou **dropper des frames** (on reste temps réel). En surveillance, on droppe.

**Frame skipping / sampling**
Ne traiter qu'une frame sur N. Le levier de perf le plus simple et le plus efficace en analyse sémantique — le contenu sémantique change rarement à 30 Hz.

---

## 9. Préprocessing

**Resize**
Redimensionnement vers la résolution d'entrée du modèle. La méthode d'interpolation (bilinéaire, bicubique) doit correspondre à celle de l'entraînement, sinon la précision chute.

**Letterbox**
Redimensionner en préservant le ratio d'aspect et compléter par des bandes de remplissage. Évite la déformation, contrairement à un resize direct.

**Normalisation (mean / std)**
Recentrer et réduire les pixels : `(pixel/255 − mean) / std`. **Les valeurs sont spécifiques au modèle** — celles de CLIP ne sont pas celles d'ImageNet. Erreur classique et silencieuse.

**BGR / RGB**
Ordre des canaux couleur. OpenCV travaille en **BGR**, la quasi-totalité des modèles attend du **RGB**. Oublier l'inversion donne un modèle qui « marche » mais avec des scores dégradés sans erreur visible.

**ROI (Region Of Interest)**
Sous-zone de l'image que l'on traite seule, pour économiser du calcul ou ignorer une zone non pertinente.

---

## 10. Sémantique & embeddings

**Embedding**
Vecteur dense de dimension fixe (512, 768…) représentant le contenu sémantique d'une entrée. La proximité géométrique entre deux embeddings reflète leur proximité de sens.

**CLIP**
Modèle à deux encodeurs — un image, un texte — entraînés à projeter leurs entrées dans un **espace latent partagé**. C'est ce qui permet de comparer directement une image et une phrase.

**Espace latent partagé**
L'espace vectoriel commun où atterrissent image et texte. Le fondement technique des filtres en langage naturel de LiveSemantic.

**Zero-shot**
Classifier selon des catégories jamais vues à l'entraînement, en décrivant simplement la classe en texte. Pas de réentraînement, pas de dataset d'exemples.

**Similarité cosinus**
Le cosinus de l'angle entre deux vecteurs, dans `[-1, 1]`. La mesure de proximité sémantique standard — insensible à la norme, donc à l'« intensité » du signal.

**Normalisation L2**
Ramener chaque embedding à une norme de 1. Une fois fait, la similarité cosinus se réduit à un simple produit scalaire — beaucoup plus rapide.

**Logit scale / température**
Facteur multiplicatif appliqué aux similarités avant le softmax. CLIP l'embarque comme paramètre appris ; l'ignorer donne des scores de confiance mal calibrés.

**Seuil de confiance (threshold)**
La valeur de similarité à partir de laquelle on déclenche un match. Le curseur principal entre faux positifs et faux négatifs.

**Vector store**
Base de données spécialisée dans la recherche par similarité sur des embeddings.

**ANN (Approximate Nearest Neighbor)**
Recherche des voisins les plus proches en acceptant une petite marge d'erreur pour gagner des ordres de grandeur en vitesse.

**HNSW**
Structure d'index ANN par graphe hiérarchique. L'algorithme par défaut de la plupart des vector stores.

---

## 11. Détection & suivi

**Bounding box**
Rectangle englobant un objet détecté, en `(x, y, largeur, hauteur)` ou `(x1, y1, x2, y2)` selon la convention — vérifier laquelle sort du modèle.

**IoU (Intersection over Union)**
Aire d'intersection divisée par aire d'union entre deux boîtes. La mesure de recouvrement standard.

**NMS (Non-Maximum Suppression)**
Post-traitement supprimant les détections redondantes : parmi les boîtes qui se recouvrent au-delà d'un seuil d'IoU, on ne garde que la plus confiante.

**Track / tracking**
Associer les détections d'une frame à l'autre pour donner une identité persistante à chaque objet. Transforme des détections isolées en trajectoires.

**Track ID**
L'identifiant stable attribué à un objet suivi à travers les frames.

**SORT / ByteTrack / DeepSORT**
Algorithmes de tracking. **SORT** utilise filtre de Kalman + IoU, léger et rapide. **ByteTrack** exploite aussi les détections à faible score. **DeepSORT** ajoute un descripteur d'apparence, plus robuste aux occlusions mais plus coûteux.

**Occlusion**
Un objet masqué temporairement. Le cas qui fait perdre son ID à un tracker naïf.

---

## 12. Go & intégration

**CGo**
Le pont permettant à Go d'appeler du C. Nécessaire pour ONNX Runtime. Coût : compilation plus lente, cross-compilation compliquée, et le passage de frontière Go↔C n'est pas gratuit — à ne pas faire dans une boucle serrée.

**Binding**
La couche Go qui enveloppe l'API C d'une bibliothèque. Ici `onnxruntime_go`.

**Linking statique / dynamique**
En *dynamique*, le binaire cherche le `.so`/`.dll`/`.dylib` au lancement — il faut le distribuer à côté. En *statique*, tout est dans le binaire : distribution simple, mais compilation nettement plus délicate avec ORT.

**Cross-compilation**
Compiler depuis une plateforme vers une autre. Trivial en Go pur, **beaucoup plus contraignant dès que CGo entre en jeu** — il faut une toolchain C pour la cible.

**Goroutine / channel**
Les primitives de concurrence de Go. Le pipeline vidéo se modélise naturellement en étages reliés par des channels : décodage → préprocessing → inférence → alerting.

**Worker pool**
Un nombre fixe de goroutines consommant une file de travail. Borne la concurrence — critique ici, car lancer une inférence par frame sans limite sature le GPU.

**Channel bufferisé**
Un channel avec une capacité. Sa taille fixe la profondeur de buffer entre étages, donc l'arbitrage latence/robustesse aux à-coups.

---

## 13. Clean architecture (rappel projet)

**Entity / domaine**
Les objets métier purs — `Frame`, `Match`, `Filter`, `Track`. Aucune dépendance externe, aucun import d'infrastructure.

**Use case**
Un scénario applicatif orchestrant le domaine : « analyser un flux et émettre des alertes ». Ne connaît que des interfaces.

**Port**
L'interface définie **par** le domaine et exprimant un besoin : `Embedder`, `VideoSource`, `AlertSender`.

**Adapter**
L'implémentation concrète d'un port dans l'infrastructure : `ONNXEmbedder`, `RTSPSource`, `WebhookAlerter`.

**Inversion de dépendance**
La règle centrale : l'infrastructure dépend du domaine, jamais l'inverse. C'est ce qui permet de remplacer ORT par Triton sans toucher au métier.

**Injection de dépendance**
Fournir les adapters au moment du câblage (`main.go`) plutôt que de les instancier au cœur du code.