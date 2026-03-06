#!/bin/sh
# Wrapper pour l'autostart : force l'environnement de session (systray)
# L'autostart XFCE peut ne pas hériter de DBUS_SESSION_BUS_ADDRESS

# Répertoire runtime utilisateur (session)
RUNTIME="${XDG_RUNTIME_DIR:-/run/user/$(id -u)}"

# Charger l'environnement de session si disponible (systemd/display manager)
if [ -r "$RUNTIME/environment" ]; then
    set -a
    . "$RUNTIME/environment"
    set +a
fi

# Bus D-Bus de session (requis pour Status Notifier / systray)
if [ -z "$DBUS_SESSION_BUS_ADDRESS" ] && [ -S "$RUNTIME/bus" ]; then
    export DBUS_SESSION_BUS_ADDRESS="unix:path=$RUNTIME/bus"
fi

# Display (au cas où)
if [ -z "$DISPLAY" ]; then
    export DISPLAY=:0
fi

# Attendre que le panneau soit prêt
sleep 10

exec /usr/local/bin/xencelabs-quick-keys "$@"
