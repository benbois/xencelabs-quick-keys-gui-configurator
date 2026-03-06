#!/bin/sh
# Affiche les infos Xencelabs Quick Keys (batterie, statut, layer)
# Raccourci clavier : Paramètres > Clavier > Raccourcis > /usr/local/bin/xencelabs-quick-keys-show-info

STATUS="${XDG_STATE_HOME:-$HOME/.local/state}/xencelabs-quick-keys/status.txt"

if [ -f "$STATUS" ]; then
	notify-send -t 5000 "Xencelabs Quick Keys" "$(tail -n +3 "$STATUS")"
else
	notify-send -t 3000 "Xencelabs Quick Keys" "En attente de connexion du périphérique."
fi
