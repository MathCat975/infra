## Load Balancer HTTP – Projet Go

Ce dépôt contient un petit **load balancer HTTP écrit en Go** (`load balancer/main.go`) et un **serveur web de test** (`server web/soft-secondaire.go`) permettant de simuler plusieurs backends avec de la latence aléatoire.

Le load balancer :
- lit la liste des backends dans `load balancer/config.yaml` ;
- mesure et **met en cache la latence** de chaque backend ;
- trie dynamiquement les backends par latence croissante ;
- envoie la requête au backend le plus rapide disponible ;
- renvoie directement la réponse HTML du backend au client.

Le serveur secondaire :
- écoute sur le port 80 ;
- introduit un **délai aléatoire** configurable ;
- renvoie une petite page HTML avec l’`id` demandé et une valeur aléatoire.

---

## 1. Pré‑requis

- Go (version récente, par exemple Go 1.20+)
- Environnement Linux (ou binaire cross‑compilé pour Linux, voir ci‑dessous)
- Accès à plusieurs machines/VM pour les backends et le serveur de load balancing (voir `doc.md` pour le détail de l’infra réseau et du firewall).

---

## 2. Compilation des binaires

Depuis chaque dossier (`load balancer` et `server web`) :

```bash
set GOOS=linux
set GOARCH=amd64
go build -o main      # ou secondaire pour le client
```

Pour le serveur secondaire (backend) :

```bash
cd "server web"
go build -o secondaire
```

Pour le load balancer :

```bash
cd "load balancer"
go build -o main
```

Copier ensuite les binaires sur les machines cibles (par exemple via un ISO comme décrit dans `doc.md`).  
Ne pas oublier de les rendre exécutables :

```bash
chmod +x main
chmod +x secondaire
```

---

## 3. Configuration des backends

La configuration des serveurs se fait via `load balancer/config.yaml` :

```yaml
servers:
  - 10.10.1.2:80
  - 10.10.2.2:80
```

Chaque entrée doit être de la forme `IP:PORT`.  
Adapter les adresses en fonction de votre topologie (par exemple celles configurées via Netplan dans `doc.md`).

---

## 4. Lancement des serveurs secondaires

Sur chaque machine « backend » (VM serveur secondaire) :

```bash
./secondaire --delay-min 50 --delay-max 500
```

Paramètres :
- `--delay-min` : délai minimum en millisecondes
- `--delay-max` : délai maximum en millisecondes

Le serveur répond sur `http://<ip-backend>:80/?id=<valeur>` avec une petite page HTML.

---

## 5. Lancement du load balancer

Sur la machine qui fait office de **load balancer** :

```bash
cd "load balancer"
./main --timeout 2000 --cache-ttl 10
```

Paramètres :
- `--timeout` : timeout HTTP vers les backends en millisecondes (par défaut 2000)
- `--cache-ttl` : durée de vie (en secondes) du cache de latence (par défaut 10)

Le load balancer écoute par défaut sur `:80` et affiche dans les logs :
- la liste des backends chargés ;
- les latences « warmup » mesurées au démarrage ;
- la latence mise en cache ;
- le backend choisi pour chaque requête.

---

## 6. Utilisation

Une fois les backends et le load balancer démarrés, envoyer des requêtes vers le load balancer :

```bash
curl "http://<ip-load-balancer>/?id=test"
```

Le load balancer :
1. trie les backends en fonction de leur latence connue (ou d’une valeur par défaut s’ils ne sont pas encore mesurés) ;
2. tente les backends dans cet ordre jusqu’à obtenir une réponse ;
3. renvoie la réponse HTML du backend au client ;
4. met à jour la latence en cache pour ce backend.

En cas d’échec de tous les backends, le load balancer renvoie `502 all backends failed`.

---

## 7. Topologie réseau et firewall

La mise en place détaillée des machines virtuelles, des adresses IP, des routes et des règles `ufw` est décrite dans `doc.md`.  
On y trouve notamment :
- la création des VMs sous `Ubuntu Server 22.04.3` ;
- la configuration Netplan des interfaces internes `10.10.x.x/30` ;
- la configuration du firewall (`ufw`) côté clients et côté serveur de load balancing ;
- les tests de connectivité (`ping`) pour vérifier que les clients ne peuvent pas se joindre entre eux mais peuvent joindre le serveur.

Utiliser `doc.md` comme référence pour reproduire l’environnement réseau complet.

---

## 8. Structure du dépôt

- `README.md` : ce fichier
- `doc.md` : documentation détaillée de l’infrastructure (VM, réseau, firewall, etc.)
- `load balancer/main.go` : code source du load balancer HTTP
- `load balancer/config.yaml` : configuration des backends
- `server web/soft-secondaire.go` : code source du serveur secondaire (backend de test)

