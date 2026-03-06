# Xencelabs Quick Keys Driver for Linux (Go)

A lightweight, userland driver for the Xencelabs Quick Keys remote, written in Go. This driver uses `uinput` to simulate keyboard events at the kernel level, making it fully compatible with **Wayland**, **X11**, and **TTYs**.

## Features

- **Full Remapping:** Map buttons and the wheel to any keyboard combination.
- **Layer Support:** Create multiple layers (e.g., General, DaVinci, Blender) and cycle through them.
- **OLED Control:** Custom text labels per button and temporary text overlays when switching layers.
- **LED Control:** Custom colored LED ring per layer.
- **Wheel Settings:** Configurable sensitivity/speed per layer.
- **Battery Monitoring:** Reports battery percentage to the log.

## Prerequisites

You need the C headers for USB and HID libraries to compile the project.

**Debian/Ubuntu:**
```bash
sudo apt install libusb-1.0-0-dev libudev-dev pkg-config
```

## Installation

1. **Clone the repository** (or create the files):
   ```bash
   go build .
   ```

### Build options

- **Basic** (no systray): `go build .`
- **With systray + Wails GUI** (recommended): `make build-tray` ou `wails build -tags "tray,webkit2_41,wails"`
  - **Important** : le tag `wails` est requis (utilise slytomcat/systray, DBus uniquement, sans conflit GTK)
  - Icône systray avec statut, layer, batterie ; menu « Configurer... » ouvre la fenêtre Wails
  - Fenêtre Wails (HTML/JS) pour éditer la configuration
  - Option `-show-gui` ou `--show-gui` : affiche la fenêtre de configuration au démarrage
  - Dépendances Wails sur Linux : `sudo apt install libgtk-3-dev libwebkit2gtk-4.1-dev`
  - Systray : si l'icône n'apparaît pas (GNOME ancien), installer `snixembed` ou un proxy StatusNotifierItems
  - Ubuntu 24.04+ : utiliser le tag `webkit2_41` (WebKitGTK 4.1)
  - Wails CLI : `go install github.com/wailsapp/wails/v2/cmd/wails@latest`

## Configuration (`config.yaml`)

The behavior of the device is defined in `config.yaml`.

### Button Layout
```text
[0]  [1]
[2]  [3]
[4]  [5]
[6]  [7]

[8] = Physical button 
[9] = Wheel Center Click
```

### Example Config Fragment
```yaml
device:
  brightness: medium      # off, low, medium, full
  orientation: 0          # 0, 90, 180, 270
  wheel_speed: normal     # global default
  overlay_duration: 2     # seconds
  keyboard_layout: azerty # qwerty or azerty (fr-FR)

layers:
  - name: "General"
    color: { r: 192, g: 192, b: 192 } # Light gray ring
    wheel_speed: fastest            # Override speed for this layer
    buttons:
      0:
        label: "Copy"
        keys: ["KEY_LEFTCTRL", "KEY_C"]
      8:
        label: "Layer"
        keys: ["INTERNAL_LAYER_CYCLE"] # Special command to switch layers
    wheel:
      left: ["KEY_VOLUMEDOWN"]
      right: ["KEY_VOLUMEUP"]
```

## Mode sans fil (Wireless)

Si vous utilisez le Quick Keys **sans fil** (dongle USB) et que l'écran OLED affiche « Please connect the computer and install driver », la télécommande n'est **pas encore appairée** au dongle.

**Appairage requis (une seule fois) :**
1. Connectez le Quick Keys au PC **avec le câble USB** (pas le dongle)
2. Sur un PC **Windows ou Mac**, installez le pilote officiel Xencelabs
3. Ouvrez l'application Xencelabs → Paramètres (icône engrenage) → « Manage Wireless Pairing »
4. Branchez le dongle USB, puis appairez la télécommande au dongle
5. Une fois appairé, le Quick Keys fonctionnera en sans fil avec ce driver Linux

**Alternative :** Utilisez le Quick Keys en **mode filaire** en le connectant directement au PC avec le câble USB. Aucun appairage nécessaire.

## Running the Driver

### 1. Udev Rules (Required)
Allow non-root access to the raw HID device. Create `/etc/udev/rules.d/50-xencelabs.rules`:

See the example file in the repo.

Reload rules: `sudo udevadm control --reload-rules && sudo udevadm trigger`

### 2. Manual Run
Since this driver creates a virtual keyboard, it requires access to `/dev/uinput`.

```bash
./xencelabs-quick-keys
```

### 3. Installation système (démarrage automatique)

```bash
# Compiler et installer le binaire + service systemd
make install

# Installer les règles udev (nécessite sudo)
sudo make udev

# IMPORTANT : permettre le démarrage des services utilisateur au boot (sans connexion)
loginctl enable-linger $USER

# Activer le service
systemctl --user daemon-reload
systemctl --user enable --now xencelabs-quick-keys
```

Sans `loginctl enable-linger`, le service ne démarre qu'à la connexion graphique. Avec linger, il démarre au boot.

Commandes utiles :
- `systemctl --user status xencelabs-quick-keys` — état du service
- `journalctl --user -u xencelabs-quick-keys -f` — afficher les logs en direct

## Supported Key Codes

Use standard Linux input event codes in `config.yaml`. Common examples:

- `KEY_A` ... `KEY_Z`
- `KEY_1` ... `KEY_0`
- `KEY_F1` ... `KEY_F12`
- `KEY_LEFTCTRL`, `KEY_LEFTSHIFT`, `KEY_LEFTALT`, `KEY_LEFTMETA` (Windows/Command)
- `KEY_ENTER`, `KEY_ESC`, `KEY_TAB`, `KEY_SPACE`, `KEY_BACKSPACE`
- `KEY_UP`, `KEY_DOWN`, `KEY_LEFT`, `KEY_RIGHT`
- `KEY_PAGEUP`, `KEY_PAGEDOWN`, `KEY_HOME`, `KEY_END`
- `KEY_VOLUMEDOWN`, `KEY_VOLUMEUP`, `KEY_MUTE`

## Debug

Pour diagnostiquer les entrées (ex: bouton central de la molette) :
```bash
XENCELABS_DEBUG=1 ./xencelabs-quick-keys
```
Affiche k1, k2, wheel en hex à chaque pression.

## Known Bugs

The overlay showing the current layer may require testing; improvements added (delays, full 32-char support).

## Acknowledgments

- Base code inspired by the work of [akhenakh](https://github.com/akhenakh/xencelabs-quick-keys-go).
- The graphical interface uses [Wails](https://wails.io/), a framework for building desktop applications with Go and web technologies.
