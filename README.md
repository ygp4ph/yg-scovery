# Yg-Scovery


Un crawler web rapide et efficace écrit en Go pour la découverte automatique de liens et l'exploration de sites web.


## Installation

### Compilation depuis les sources

```bash
# Cloner le repository
git clone <your-repo-url>
cd yg-scovery

# Installer les dépendances
go mod download

# Compiler
go build -o yg-scovery

# Ou installer directement
go install
```

## Utilisation

### Syntaxe de base

```bash
./yg-scovery -u <URL> [options]
```

### Options disponibles

| Flag | Alias | Description | Défaut |
|------|-------|-------------|--------|
| `-u` | `--url` | URL cible à crawler (requis) | - |
| `-d` | `--depth` | Profondeur maximale de récursion | 3 |
| `-e` | `--ext` | Afficher uniquement les liens externes | false |
| `-i` | `--int` | Afficher uniquement les liens internes | false |
| `-h` | `--help` | Afficher l'aide | - |

### Exemple

```bash
~/Projets/Yg-scovery $ ./yg-scovery -u https://trainplay.fr/ -i -d 2

_____.___.                  _________                                      
\__  |   | ____            /   _____/ ____  _______  __ ___________ ___.__.
 /   |   |/ ___\   ______  \_____  \_/ ___\/  _ \  \/ // __ \_  __ <   |  |
 \____   / /_/  > /_____/  /        \  \__(  <_> )   /\  ___/|  | \/\___  |
 / ______\___  /          /_______  /\___  >____/ \_/  \___  >__|   / ____|
 \/     /_____/                   \/     \/                \/       \/      v1.0.0


[INF] Scanning https://trainplay.fr/ (Depth: 2)
[INF] Filter: Internal links only
[INT] https://trainplay.fr/main.js
[INT] https://trainplay.fr
[INT] https://trainplay.fr/main.js
[INT] https://trainplay.fr/dist/
[INT] https://trainplay.fr/dist/main.js
[INT] https://trainplay.fr/main.js
[INT] https://trainplay.fr/
[INT] https://trainplay.fr/register
[INT] https://trainplay.fr/login
[INT] https://trainplay.fr/img/logo.jpg
[INT] https://trainplay.fr/MyNews
[INT] https://trainplay.fr/g,s=
[INT] https://trainplay.fr/users/authenticate
[INT] https://trainplay.fr/users/register
[INT] https://trainplay.fr/users
[INT] https://trainplay.fr/users/
[INT] https://trainplay.fr/
[INT] https://trainplay.fr/%3e%3c/svg%3e
[INT] https://trainplay.fr/g,
[INT] https://trainplay.fr/page
[INT] https://trainplay.fr/a/i
[INT] https://trainplay.fr/a/b
```
