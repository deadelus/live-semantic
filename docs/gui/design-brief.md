# Brief design — LiveSemantic (GUI desktop + web)

Document à transmettre pour la conception des maquettes. Se concentre sur le
produit, les écrans et les interactions — pas sur l'architecture technique
(voir `docs/gui/spec.md` pour les prérequis backend, si besoin de contexte
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

## Structure générale (un seul écran de départ, pas des écrans parallèles)

Il n'y a pas plusieurs "écrans" indépendants entre lesquels on navigue —
une seule structure :

1. **Écran d'accueil "Sources"** (liste ou mosaïque, § ci-dessous) : c'est
   là qu'on ajoute une source, qu'on voit son statut, et qu'on clique
   dessus.
2. **Cliquer sur une tuile/ligne ouvre un onglet.** Cet onglet affiche la
   **Vue live** de cette source (vidéo + boxes + volet de droite, § plus
   bas). Plusieurs onglets peuvent être ouverts en même temps (un par
   source consultée).
3. Le panel "Sources" reste accessible (par ex. comme premier onglet
   permanent, ou via un bouton "retour") pour revenir en ajouter une autre
   ou changer de vue liste/mosaïque.

### Écran d'accueil — Sources (liste ou mosaïque)

- Liste des flux configurés — webcam locale, caméra IP/RTSP, webcam du
  navigateur, fichier vidéo local, flux YouTube/live. Ajout, suppression,
  édition.
- **Deux modes d'affichage au choix de l'utilisateur** (toggle accessible) :
  - **Vue liste** : compacte, textuelle, une ligne par source, avec statut.
  - **Vue mosaïque** : grille de tuiles vidéo. Par défaut, chaque tuile
    affiche un **aperçu léger** (~1 image/seconde, **sans** boxes ni
    overlay) pour rester fluide avec beaucoup de flux actifs.
- **Réglages de la mosaïque elle-même** (volet de droite, repliable) :
  - Afficher ou non les boxes sur les tuiles.
  - Augmenter le FPS d'affichage des tuiles (cas d'usage : surveiller 4
    caméras à la fois avec plus de détail plutôt qu'un simple aperçu).
    **Avertissement à prévoir dans l'UI** : monter le FPS/activer les boxes
    sur plusieurs tuiles en simultané peut introduire de la latence selon
    le matériel — pas une garantie de fluidité, à formuler comme un
    compromis explicite pour l'utilisateur (ex. un indicateur de charge),
    pas une simple case à cocher sans conséquence visible.
- **Clic sur une tuile ou une ligne** → ouvre un onglet avec la Vue live de
  cette source (voir ci-dessous).
- Statut par source, visible en liste comme en mosaïque : connecté /
  déconnecté / erreur / reconnexion en cours.

### Onglet — Vue live (ouvert au clic sur une source)

- Vidéo de la source, boxes dessinées dessus, **couleur des box
  configurable** par l'utilisateur (pas figée par catégorie d'objet — une
  couleur par filtre/entrée de galerie actif).
- **Score de similarité affiché en direct** sur ou à côté de chaque box
  (valeur entre 0.0 et 1.0).
- **Lecture/pause/reprise** : l'utilisateur peut mettre la vidéo en pause
  et la reprendre à tout moment, sans couper le flux réel en arrière-plan.
- **Retour en arrière (rewind)** : possibilité de revenir sur les
  dernières secondes/minutes de flux. Durée de ce buffer configurable par
  l'utilisateur (ex. "garder les 30 dernières secondes"). Techniquement,
  ça implique que le backend garde un tampon glissant de frames par flux
  — coût mémoire à prendre en compte côté implémentation (pas un détail
  gratuit), pas un problème de design.
- **Volet de droite, repliable** :
  - Liste des boxes/objets actuellement détectés sur ce flux.
  - Paramètres du flux (filtre, seuil avec retour visuel en direct — voir
    ci-dessous).
  - Section "avancés", dépliable en dessous des paramètres principaux
    (repliée par défaut, public technique) : paramètres de tracking fins,
    choix du matériel d'inférence, choix de variante de modèle.
- **Interaction clé — sélection runtime + labellisation** : l'utilisateur
  clique sur une box déjà détectée, ou dessine une box à main levée si rien
  n'est détecté dessus → un petit formulaire lui demande un label → l'objet
  rejoint une galerie de références réutilisable, listée dans le volet de
  droite. Chaque entrée : activable/désactivable, renommable, supprimable
  individuellement. Doit rester fluide, sans bloquer longtemps la lecture.
- Réglages de filtre dans le volet de droite :
  - Champ de filtre texte libre — plusieurs filtres actifs simultanément
    possibles sur un même flux.
  - **Slider de seuil de similarité avec retour visuel en direct** : le
    score mesuré doit visiblement bouger pendant que l'utilisateur déplace
    le slider, avant même de valider. Élément différenciant du produit,
    pas un simple champ numérique — à soigner particulièrement.
  - Raccourci "appliquer à toutes les sources actives" (utile en usage
    mono-caméra ou pour répliquer un réglage sur plusieurs flux d'un coup).
- Overlay de performance optionnel et discret : FPS, latence, nombre
  d'objets suivis — masquable, ne doit jamais gêner la lecture de l'image.

### Historique / alertes

- Journal des événements (objet détecté / confirmé / perdu) avec
  timestamp, source, label, score.
- Filtrable par caméra/source. Accessible depuis l'accueil ou depuis
  chaque onglet (à trancher avec le design).

---

## Décisions UX actées (2026-08-10)

Ces points sont tranchés, pas à re-questionner par le design :

- **Thème clair/sombre au choix de l'utilisateur**, toggle accessible (pas
  enfoui dans un sous-menu) — concevoir les deux variantes complètes, pas
  seulement un thème sombre par défaut.
- **Affichage multi-flux : liste OU mosaïque, au choix de l'utilisateur**
  (toggle) — pas un seul mode imposé.
- **Mosaïque = aperçu léger par défaut** (~1 fps, sans boxes/overlay),
  mais **réglable par l'utilisateur** (FPS plus élevé, boxes visibles sur
  les tuiles) via le volet de droite de l'accueil — avec avertissement de
  latence possible selon le matériel. Clic sur une tuile/ligne → ouvre un
  **onglet** avec la Vue live complète (framerate normal, boxes, scores,
  volet de droite) — ce n'est pas un "écran" séparé, c'est ce qui s'affiche
  dans l'onglet.
- **Couleur des box configurable par l'utilisateur**, pas figée par
  catégorie d'objet détecté.
- **Sélection + labellisation en direct** pendant que le flux tourne (pas
  uniquement un filtre texte saisi avant de lancer la session).
- **Lecture/pause/reprise + retour en arrière (rewind)** sur la Vue live,
  avec une durée de buffer configurable par l'utilisateur.

---

## Contraintes de design

- La vidéo est **toujours** l'élément dominant de l'écran — aucun overlay
  ne doit gêner la lecture de l'image ni cacher un objet suivi.
- Densité d'info élevée mais lisible en un coup d'œil (usage surveillance =
  scan rapide, pas lecture posée).
- Prévoir clairement les états dégradés : "aucun flux configuré", "flux en
  erreur", "flux en reconnexion" — l'app doit rester lisible même en usage
  dégradé (coupure réseau, caméra IP indisponible).
- Le slider de seuil avec feedback live et le flux de sélection +
  labellisation (dans la Vue live) sont les deux interactions les plus
  différenciantes du produit — à traiter avec le plus de soin.

---

## Livrables attendus

- Wireframes ou maquettes moyenne/haute fidélité pour : l'accueil Sources
  (variantes liste et mosaïque, y compris son volet de réglages), et
  l'onglet Vue live (avec son volet de droite et sa section avancés
  dépliée/repliée) — en variante claire ET sombre.
- Flux d'interaction complet, étape par étape, pour :
  - la sélection + labellisation d'un objet en direct (Vue live),
  - le passage accueil (mosaïque/liste) → onglet Vue live au clic sur une
    source,
  - la pause/reprise/rewind de la Vue live.
- Système de composants réutilisables si possible (boutons, cartes de
  source, badges de statut, box vidéo overlay, slider avec preview),
  pensé pour tenir en web ET desktop sans changement.
