# Tracking-by-detection — comparatif des trackers gocv et décision

> **Statut** : référence technique + ADR
> **Portée** : choix du tracker mono-objet pour la boucle de ré-ancrage (TODO.md § B)
> **Dernière révision** : 2026-08-06

---

## 1. Contexte

TODO.md § B prévoyait un choix entre **KCF, CSRT ou MOSSE**, "aucun des trois
n'étant présupposé meilleur ici", à trancher par un test de dérive sur vidéo
réelle. Avant de lancer ce test, vérification de ce que le binding Go expose
réellement — l'hypothèse de départ s'est révélée partiellement fausse.

## 2. Ce que `gocv v0.42.0` expose réellement

Vérifié directement dans le module (`$(go env GOMODCACHE)/gocv.io/x/gocv@v0.42.0`),
pas supposé depuis la doc OpenCV générale :

| Tracker | Package Go | Dépendance native | Modèle externe requis |
|---|---|---|---|
| **KCF** | `gocv.io/x/gocv/contrib` | `opencv_tracking` (module contrib) | Non |
| **CSRT** | `gocv.io/x/gocv/contrib` | `opencv_tracking` (module contrib) | Non |
| **MIL** | `gocv.io/x/gocv` (core, `video.go`) | `opencv2/video.hpp` (toujours présent) | Non |
| **MOSSE** | **absent** | — | — |
| GOTURN | `gocv.io/x/gocv` (core) | `opencv2/video.hpp` | Oui — `.caffemodel` + `.prototxt` |
| ViT-tracker | `gocv.io/x/gocv` (core) | `opencv2/video.hpp` | Oui — modèle ONNX dédié |

**MOSSE n'existe plus dans le binding.** Le `CHANGELOG.md` de gocv le confirme
explicitement : *"change MOSSE to KCF"* — cohérent avec l'historique OpenCV
(`cv::TrackerMOSSE` déprécié/retiré de l'API `tracking` moderne au profit de
KCF, la variante legacy n'étant pas bindée par gocv). **La prémisse de
TODO.md § B est donc à corriger : le choix réel est KCF vs CSRT vs MIL**, pas
KCF vs CSRT vs MOSSE. GOTURN et le tracker ViT sont écartés d'emblée : ils
demandent chacun de sourcer un modèle externe supplémentaire (même problème
de poids/provenance que `yolo11s.onnx`), hors scope pour "juste l'intégration"
visée par cet item.

## 3. Le module contrib est-il seulement disponible sur ce projet ?

Vérifié sur la machine de dev (macOS, `brew install opencv`, cohérent avec
`readme.md` § Installation) :

```
$ pkg-config --modversion opencv4
4.12.0
$ pkg-config --libs opencv4 | tr ' ' '\n' | grep tracking
-lopencv_tracking
$ ls $(brew --prefix opencv)/include/opencv4/opencv2/tracking/
tracking.hpp  tracking_legacy.hpp  tracking_internals.hpp  ...
```

Le module `opencv_tracking` est bien présent dans la formule Homebrew
`opencv` standard — pas d'étape d'installation supplémentaire à documenter
pour KCF/CSRT sur macOS. **Non vérifié** : Linux (dépend de la distro/du
packaging OpenCV utilisé en CI/prod, à confirmer avant de fermer Décision H).
Le build tag de `gocv.io/x/gocv/contrib` inclut le support tracking par
défaut (`!gocv_specific_modules || (... && gocv_contrib_tracking)`) — aucun
flag de build spécial requis dans ce projet (`gocv_specific_modules` n'est
utilisé nulle part ici).

## 4. Comparatif KCF / CSRT / MIL

| Critère | KCF | CSRT | MIL |
|---|---|---|---|
| Vitesse (relatif) | Rapide | Plus lent que KCF (~3-5×, filtre plus riche) | Lent (classificateur en ligne) |
| Robustesse à l'occlusion partielle | Moyenne | Meilleure (feature channels multiples + reliability map) | Moyenne |
| Robustesse au changement d'échelle | Faible-moyenne | Meilleure | Faible |
| Robustesse à la rotation | Faible | Faible-moyenne | Faible |
| Risque de dérive silencieuse (perd la cible sans le signaler) | Connu pour ça sur mouvement rapide | Moins fréquent | Fréquent, tracker plus ancien/fragile |
| Coût CPU en boucle temps réel | Le moins cher des trois | Le plus cher des trois | Intermédiaire |
| API gocv | `contrib.NewTrackerKCF()` | `contrib.NewTrackerCSRT()` | `gocv.NewTrackerMIL()` |

Chiffres qualitatifs issus de la littérature OpenCV/benchmarks tiers — **pas
mesurés sur ce projet**, c'est précisément l'objet du test de dérive prévu en
TODO.md § B (vidéo réelle, mêmes conditions que la webcam cible, mesure de la
dérive sur occlusion/sortie de cadre/changement d'échelle).

## 5. Décision

- **Candidats retenus pour le test de dérive** : KCF et CSRT (contrib, déjà
  disponibles sans dépendance supplémentaire sur ce poste de dev). MIL reste
  en réserve si KCF/CSRT déçoivent tous les deux — coût d'intégration nul
  puisqu'il suit la même interface `gocv.Tracker`.
- **MOSSE abandonné** : n'existe pas dans le binding utilisé (`gocv v0.42.0`),
  pas de fork/contribution amont prévue pour cet effort XS-M.
- **GOTURN / ViT-tracker écartés** : nécessitent un modèle externe, hors
  scope de "l'intégration" visée ici — à reconsidérer seulement si KCF/CSRT/MIL
  s'avèrent tous insuffisants sur la vidéo réelle.

## 6. Impact backlog

TODO.md § B mis à jour en conséquence : `MOSSE` remplacé par `MIL` dans
l'item "Intégrer un tracker mono-objet gocv".

## 7. Résultats du test de dérive (2026-08-09)

Outil : `cmd/tracking-drift-bench` (headless, pas de fenêtre — voir godoc du
fichier pour la méthodologie complète). Principe : suivre un objet unique
avec le tracker seul entre deux re-détections YOLO, mesurer l'IoU entre la
position du tracker et la re-détection fraîche à chaque point de contrôle
(c'est la dérive), avant de recaler le tracker dessus.

| Vidéo | Classe | Intervalle | Checkpoints | KCF avg IoU | KCF min | KCF échecs `Update()` | CSRT avg IoU | CSRT min | CSRT échecs `Update()` |
|---|---|---|---|---|---|---|---|---|---|
| `person.mp4` (32s, 956 frames) | person | 45 (défaut prod) | 21 | **0.723** | 0.325 | 92 | 0.636 | 0.000 | 0 |
| `person.mp4` | person | 30 | 31 | 0.695 | 0.000 | 122 | 0.680 | 0.000 | 0 |
| `car.mp4` (8s, 238 frames) | truck | 30 | 6 | **0.240** | 0.060 | 0 | 0.194 | 0.105 | 0 |
| `car.mp4` | car | 30 | 6 | 0.000 | 0.000 | 57 | 0.140 | 0.140 | 0 |

**Limite de méthodologie découverte en cours de route** : sur `car.mp4`,
YOLO alterne le label du même véhicule entre `car` et `truck` d'une frame à
l'autre (ambiguïté COCO classique sur les utilitaires/pickups). L'outil
associe strictement par classe identique — un flip de label casse
l'association et fait chuter l'IoU mesuré sans que ce soit une vraie
dérive du tracker. La ligne `car.mp4`/classe `car` est donc **peu fiable**,
gardée ici pour transparence plutôt que retirée. La ligne `truck` est plus
propre (label stable sur la fenêtre observée) et reste le signal le plus
exploitable pour cette vidéo.

**Lecture** :
- **KCF devance CSRT sur l'IoU moyen dans tous les cas exploitables**,
  contrairement à l'intuition "CSRT plus robuste" de la littérature
  générale (§ 4) — mesuré, pas supposé, sur ce jeu de données précis.
- **CSRT ne signale (presque) jamais d'échec** (`Update()` ne renvoit
  quasiment jamais `false`) mais peut dériver silencieusement jusqu'à IoU
  0.000 (`person.mp4`) — KCF échoue plus souvent en apparence mais l'échec
  est un signal exploitable (déclenche `Miss()` → re-ancrage plus tôt dans
  `trackManager`), alors que le silence de CSRT ne l'est pas.
- Échantillon **petit** (2 vidéos, un seul objet suivi par run, pas de
  répétition) — suffisant pour trancher un défaut raisonnable, pas pour
  clore le sujet définitivement.

## 8. Décision finale

**KCF reste le défaut** (déjà câblé dans `main.go` avant ce test) — les
mesures ci-dessus le confirment plutôt que le contredisent. CSRT reste
disponible en un changement d'une ligne (`gocvtracker.New(gocvtracker.CSRT)`)
si un usage futur avec occlusions plus franches montre l'inverse. TODO.md
§ B, item "test de dérive", coché en conséquence.

## 9. Références

- OpenCV Tracking API (module `tracking`, contrib) : https://docs.opencv.org/4.x/d9/df8/group__tracking.html
- gocv contrib tracking bindings : `gocv.io/x/gocv/contrib` (`tracking.go`, `tracking.h`, `tracking.cpp`)
- gocv core tracking bindings (MIL/GOTURN/ViT) : `gocv.io/x/gocv` (`video.go`, `video.h`)
- gocv CHANGELOG, entrée MOSSE → KCF (confirme le retrait de MOSSE du binding)
