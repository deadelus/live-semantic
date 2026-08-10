# Brief design — LiveSemantic (GUI desktop + web)

Document à transmettre pour la conception des maquettes. Se concentre sur le
produit, les écrans et les interactions — pas sur l'architecture technique
(voir `docs/gui-spec.md` pour les prérequis backend, si besoin de contexte
technique).

## Contexte produit

LiveSemantic est une application de vidéosurveillance intelligente temps
réel : plusieurs flux vidéo sont analysés en continu par un pipeline de
détection d'objets + reconnaissance sémantique par IA. L'utilisateur définit
ce qu'il cherche soit en texte libre ("une personne", "un sac abandonné"),
soit en sélectionnant un objet à l'écran et en le labellisant lui-même
pendant que le flux tourne. Quand un objet correspond, l'app le suit
(tracking) et alerte.

## Utilisateurs / cas d'usage

- Usage multi-caméra type surveillance/monitoring : plusieurs flux en
  simultané, sessions longues, besoin de repérer vite un événement dans un
  ensemble de flux.
- Usage mono-caméra simple : un utilisateur qui teste avec sa webcam, filtre
  unique, session courte.

## Plateformes

Un seul design, deux déploiements : application web (navigateur) et
application desktop (wrapper natif autour de la même UI web — même code,
même look). Le design doit être pensé "web app" (barre d'actions, densité
d'info proche d'un outil pro type Grafana / vidéosurveillance IP / studio de
streaming), pas "site web".

---

## Écrans

### Écran 1 — Vue live

- Flux vidéo temps réel, boxes dessinées dessus.
- **Couleur des box configurable** par l'utilisateur — pas une couleur figée
  par catégorie d'objet, une couleur par filtre/entrée de galerie actif.
- **Score de similarité affiché en direct** sur ou à côté de chaque box
  (valeur entre 0.0 et 1.0).
- Overlay de performance optionnel et discret : FPS, latence, nombre
  d'objets suivis — masquable, ne doit jamais gêner la lecture de l'image.
- **Interaction clé — sélection runtime + labellisation** : l'utilisateur
  clique sur une box déjà détectée, ou dessine une box à main levée si rien
  n'est détecté dessus → un petit formulaire lui demande un label → l'objet
  rejoint une galerie de références réutilisable. Cette interaction doit
  être fluide et rapide, sans bloquer longtemps la lecture du flux.
- Panel latéral "galerie de références" : une entrée par objet labellisé
  (vignette + nom), chacune activable/désactivable, renommable, supprimable
  individuellement.

### Écran 2 — Gestion des sources / multi-flux

- Panel "Sources" : liste des flux configurés — webcam locale, caméra
  IP/RTSP, webcam du navigateur, fichier vidéo local, flux YouTube/live.
  Ajout, suppression, édition.
- **Deux modes d'affichage au choix de l'utilisateur** (toggle accessible,
  pas un seul mode imposé) :
  - **Vue liste** : compacte, textuelle, une ligne par source.
  - **Vue mosaïque** : grille de tuiles vidéo. Chaque tuile affiche un
    **aperçu léger** (rafraîchi lentement, ~1 image/seconde, **sans** boxes
    ni overlay — juste l'image brute) pour rester fluide même avec
    beaucoup de flux actifs en simultané.
- **Clic sur une tuile de la mosaïque** → bascule automatiquement vers la
  **vue onglets**, en activant l'onglet correspondant à ce flux. L'onglet
  affiche la vidéo complète telle que décrite en Écran 1 (framerate normal,
  boxes, scores, contrôles). La vue onglets devient la vue active par
  défaut après ce clic ; l'utilisateur peut revenir manuellement à la
  mosaïque ou à la liste ensuite.
- Statut par source, visible en liste comme en mosaïque : connecté /
  déconnecté / erreur / reconnexion en cours.

### Écran 3 — Panel de contrôle / filtres

- Réglages **par flux**, avec un raccourci "appliquer à toutes les sources
  actives" pour l'usage mono-caméra simple.
- Champ de filtre texte libre — plusieurs filtres actifs simultanément
  possibles sur un même flux.
- **Slider de seuil de similarité avec retour visuel en direct** : le score
  mesuré doit visiblement bouger pendant que l'utilisateur déplace le
  slider, avant même de valider. C'est un élément différenciant du produit,
  pas un simple champ numérique — à soigner particulièrement.
- Activation/désactivation individuelle de chaque entrée de la galerie de
  références (sans la supprimer).
- Sélecteur d'algorithme de tracking, sélecteur de source par flux/onglet.

### Écran 4 — Réglages avancés

Repliable/masqué par défaut (public technique). Paramètres de tracking fins
(sensibilité, délais), choix du matériel d'inférence (CPU/GPU si
disponible), choix de variante de modèle. Pas d'exigence visuelle
particulière — un formulaire classique suffit.

### Écran 5 — Historique / alertes

- Journal des événements (objet détecté / confirmé / perdu) avec
  timestamp, source, label, score.
- Filtrable par caméra/source.

---

## Décisions UX actées (2026-08-10)

Ces points sont tranchés, pas à re-questionner par le design :

- **Thème clair/sombre au choix de l'utilisateur**, toggle accessible (pas
  enfoui dans un sous-menu) — concevoir les deux variantes complètes, pas
  seulement un thème sombre par défaut.
- **Affichage multi-flux : liste OU mosaïque, au choix de l'utilisateur**
  (toggle) — pas un seul mode imposé.
- **Mosaïque = aperçu léger** (~1 fps, sans boxes/overlay), distincte de la
  **vue onglets = flux complet** (framerate normal, boxes, scores,
  contrôles). Clic sur une tuile → bascule en vue onglets automatiquement.
- **Couleur des box configurable par l'utilisateur**, pas figée par
  catégorie d'objet détecté.
- **Sélection + labellisation en direct** pendant que le flux tourne (pas
  uniquement un filtre texte saisi avant de lancer la session).

---

## Contraintes de design

- La vidéo est **toujours** l'élément dominant de l'écran — aucun overlay
  ne doit gêner la lecture de l'image ni cacher un objet suivi.
- Densité d'info élevée mais lisible en un coup d'œil (usage surveillance =
  scan rapide, pas lecture posée).
- Prévoir clairement les états dégradés : "aucun flux configuré", "flux en
  erreur", "flux en reconnexion" — l'app doit rester lisible même en usage
  dégradé (coupure réseau, caméra IP indisponible).
- Le slider de seuil avec feedback live (Écran 3) et le flux de sélection +
  labellisation (Écran 1) sont les deux interactions les plus différenciantes
  du produit — à traiter avec le plus de soin.

---

## Livrables attendus

- Wireframes ou maquettes moyenne/haute fidélité pour les 5 écrans
  ci-dessus, en variante claire ET sombre.
- Flux d'interaction complet, étape par étape, pour :
  - la sélection + labellisation d'un objet en direct (Écran 1),
  - le passage mosaïque → onglet au clic sur une tuile (Écran 2).
- Système de composants réutilisables si possible (boutons, cartes de
  source, badges de statut, box vidéo overlay, slider avec preview),
  pensé pour tenir en web ET desktop sans changement.
