# Cross-compilation

> Vérifié le 2026-08-06 en essayant réellement de cross-compiler depuis macOS vers Linux — pas une supposition. Voir `docs/adr/inference-runtimes.md` §7 pour le contexte ADR (dépendance CGo à ONNX Runtime).

## Constat : `GOOS`/`GOARCH` seuls ne suffisent pas

Le projet dépend de deux bibliothèques CGo : `gocv` (lie contre OpenCV au moment de la compilation) et `onnxruntime_go` (charge `onnxruntime.{so,dylib,dll}` dynamiquement à l'exécution, mais son propre code C nécessite CGo à la compilation). `CGO_ENABLED=1` est nécessaire pour compiler l'un ou l'autre.

Essai réel depuis macOS (arm64) vers Linux amd64 :

```bash
GOOS=linux GOARCH=amd64 go build ./cmd/livesemantic
# → build constraints exclude all Go files in onnxruntime_go
#   (CGO_ENABLED désactivé automatiquement en cross-compilation sans toolchain C définie)

CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build ./cmd/livesemantic
# → clang (macOS/Xcode) essaie de compiler du code C ciblant Linux avec les
#   headers du SDK macOS : setresgid/setresuid non déclarés (spécifiques à
#   glibc Linux, absents de libSystem macOS). Échec de compilation du runtime
#   cgo lui-même, avant même d'atteindre gocv ou onnxruntime_go.
```

**Le compilateur C de l'hôte ne sait pas produire du code pour une autre plateforme.** `go build` seul ne fournit jamais de toolchain C cross — c'est un besoin système, pas Go.

## Ce qui bloquerait même avec un toolchain C cross fonctionnel

Un vrai cross-compilateur C (ex. `zig cc`, ou un `gcc`/`clang` cross installé) résoudrait l'échec ci-dessus, mais **pas** le suivant : `gocv` lie contre les headers et bibliothèques **OpenCV compilées pour la plateforme cible**. Contrairement à `onnxruntime_go` (qui charge son `.so`/`.dylib`/`.dll` à l'exécution depuis `assets/libraries/`), OpenCV doit être présent, compilé pour la cible, **au moment du build**. Il n'existe pas d'équivalent à `assets/libraries/` pour OpenCV dans ce projet — c'est la dépendance système de l'hôte (`brew install opencv` sur macOS, `apt install libopencv-dev` sur Linux, etc.) qui est utilisée.

## Approches réalistes, par ordre de préférence

### 1. Build natif par plateforme (ce qui est fait aujourd'hui, implicitement)

Compiler directement sur (ou avec un runner CI natif pour) chaque OS/arch cible, avec OpenCV installé localement à chaque fois. Le plus simple, le plus fiable, zéro configuration cross. Défaut : autant de machines/runners que de cibles.

### 2. Build Docker multi-arch avec OpenCV installé dans l'image cible

Une image Docker par cible (`--platform linux/amd64`, `linux/arm64`, ...) avec OpenCV et le toolchain C installés dedans, buildkit/QEMU pour l'émulation si l'hôte ne matche pas nativement l'architecture. Fonctionne sans machine physique par cible, mais coût de build plus élevé (émulation) et image à maintenir par cible.

### 3. Toolchain C cross + OpenCV cross-compilé manuellement

Techniquement possible (`zig cc` comme `CC`, OpenCV compilé en cross via son propre système de build CMake), mais lourd à maintenir pour un gain marginal face à l'option 2. À envisager seulement si Docker n'est pas disponible dans l'environnement de build cible (ex. contrainte CI spécifique).

## Recommandation pour ce projet

Option 1 tant que le nombre de cibles reste petit (aujourd'hui : poste de développement macOS/Linux). Passer à l'option 2 si/quand un pipeline de release multi-plateforme devient nécessaire (voir `MIGRATION.md` Phase 3+ pour le contexte de déploiement).

## À corriger

`readme.md` contient actuellement (`## Build Commands`) :
```bash
GOOS=linux GOARCH=amd64 go build -o livesemantic-linux ./cmd/livesemantic
```
présenté comme une commande qui fonctionne telle quelle. **Elle échoue** pour les raisons ci-dessus dès qu'elle est lancée depuis une machine qui n'est pas déjà du Linux amd64 avec OpenCV installé. À corriger pour ne pas induire en erreur un nouveau contributeur.
