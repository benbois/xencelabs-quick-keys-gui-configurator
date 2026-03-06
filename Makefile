PREFIX ?= /usr/local
BINDIR = $(PREFIX)/bin
DATADIR = $(PREFIX)/share/xencelabs-quick-keys
SYSTEMD_USER = $(HOME)/.config/systemd/user
UDEV_RULES = /etc/udev/rules.d

.PHONY: build build-tray install install-icon uninstall udev

build:
	go build -o xencelabs-quick-keys .

build-tray:
	CGO_ENABLED=1 wails build -tags "tray,webkit2_41,wails"

# Installe l'icône xencelabs-icon.png et le .desktop pour la barre de titre (Wayland/GNOME)
install-icon:
	@mkdir -p $(HOME)/.local/share/icons/hicolor/256x256/apps
	@mkdir -p $(HOME)/.local/share/icons/hicolor/48x48/apps
	@mkdir -p $(HOME)/.local/share/icons/hicolor/32x32/apps
	@mkdir -p $(HOME)/.local/share/icons/hicolor/22x22/apps
	@mkdir -p $(HOME)/.local/share/icons/hicolor/16x16/apps
	@mkdir -p $(HOME)/.local/share/applications
	@cp xencelabs-icon.png $(HOME)/.local/share/icons/hicolor/256x256/apps/xencelabs-quick-keys.png
	@cp xencelabs-icon.png $(HOME)/.local/share/icons/hicolor/48x48/apps/xencelabs-quick-keys.png
	@cp xencelabs-icon.png $(HOME)/.local/share/icons/hicolor/32x32/apps/xencelabs-quick-keys.png
	@cp xencelabs-icon.png $(HOME)/.local/share/icons/hicolor/22x22/apps/xencelabs-quick-keys.png
	@cp xencelabs-icon.png $(HOME)/.local/share/icons/hicolor/16x16/apps/xencelabs-quick-keys.png
	@sed 's|/usr/local/bin/xencelabs-quick-keys|$(PREFIX)/bin/xencelabs-quick-keys|g' xencelabs-quick-keys.desktop > $(HOME)/.local/share/applications/xencelabs-quick-keys.desktop
	@gtk-update-icon-cache -f -t $(HOME)/.local/share/icons/hicolor 2>/dev/null || true
	@echo "Icône xencelabs-icon.png et .desktop installés. Relancez l'app."

install: build-tray
	install -Dm755 xencelabs-quick-keys $(BINDIR)/xencelabs-quick-keys
	install -Dm755 xencelabs-quick-keys-autostart.sh $(BINDIR)/xencelabs-quick-keys-autostart
	install -Dm755 xencelabs-quick-keys-show-info.sh $(BINDIR)/xencelabs-quick-keys-show-info
	install -Dm644 xencelabs-icon.png $(DATADIR)/xencelabs-icon.png
	@TARGET_USER=$${SUDO_USER:-$$USER}; \
	TARGET_HOME=$$(eval echo ~$$TARGET_USER); \
	mkdir -p $$TARGET_HOME/.config/systemd/user; \
	cp xencelabs-quick-keys.service $$TARGET_HOME/.config/systemd/user/; \
	if [ "$$(id -u)" -eq 0 ] && [ -n "$$SUDO_USER" ]; then \
		RUNTIME=/run/user/$$(id -u $$SUDO_USER); \
		sudo -u $$SUDO_USER XDG_RUNTIME_DIR=$$RUNTIME DBUS_SESSION_BUS_ADDRESS=unix:path=$$RUNTIME/bus systemctl --user daemon-reload && \
		sudo -u $$SUDO_USER XDG_RUNTIME_DIR=$$RUNTIME DBUS_SESSION_BUS_ADDRESS=unix:path=$$RUNTIME/bus systemctl --user enable --now xencelabs-quick-keys || \
		echo "Exécutez manuellement : systemctl --user daemon-reload && systemctl --user enable --now xencelabs-quick-keys"; \
	else \
		systemctl --user daemon-reload && systemctl --user enable --now xencelabs-quick-keys; \
	fi
	@if [ "$$(id -u)" -eq 0 ] && [ -n "$$SUDO_USER" ]; then \
		loginctl enable-linger $$SUDO_USER 2>/dev/null && echo "Linger activé pour $$SUDO_USER (démarrage au boot)"; \
	else \
		loginctl enable-linger $$USER 2>/dev/null && echo "Linger activé (démarrage au boot)"; \
	fi || true
	@echo ""
	@echo "Pour les règles udev : sudo make udev"
	@echo ""

udev:
	install -Dm644 50-xencelabs.rules $(UDEV_RULES)/50-xencelabs.rules
	udevadm control --reload-rules
	udevadm trigger
	@echo "Règles udev installées. Débranchez et rebranchez le périphérique si nécessaire."

uninstall:
	@TARGET_USER=$${SUDO_USER:-$$USER}; \
	TARGET_HOME=$$(eval echo ~$$TARGET_USER); \
	echo "Désinstallation de xencelabs-quick-keys..."; \
	rm -f $(BINDIR)/xencelabs-quick-keys $(BINDIR)/xencelabs-quick-keys-autostart $(BINDIR)/xencelabs-quick-keys-show-info 2>/dev/null || { echo "  sudo requis pour supprimer $(BINDIR)/xencelabs-quick-keys"; }; \
	rm -rf $(DATADIR) 2>/dev/null || { echo "  sudo requis pour supprimer $(DATADIR)"; }; \
	rm -f $$TARGET_HOME/.config/systemd/user/xencelabs-quick-keys.service 2>/dev/null || true; \
	if [ "$$(id -u)" -eq 0 ] && [ -n "$$SUDO_USER" ]; then \
		RUNTIME=/run/user/$$(id -u $$SUDO_USER); \
		sudo -u $$SUDO_USER XDG_RUNTIME_DIR=$$RUNTIME DBUS_SESSION_BUS_ADDRESS=unix:path=$$RUNTIME/bus systemctl --user disable --now xencelabs-quick-keys 2>/dev/null || true; \
		sudo -u $$SUDO_USER XDG_RUNTIME_DIR=$$RUNTIME DBUS_SESSION_BUS_ADDRESS=unix:path=$$RUNTIME/bus systemctl --user daemon-reload 2>/dev/null || true; \
	else \
		systemctl --user disable --now xencelabs-quick-keys 2>/dev/null || true; \
		systemctl --user daemon-reload 2>/dev/null || true; \
	fi; \
	rm -f $(UDEV_RULES)/50-xencelabs.rules 2>/dev/null || { echo "  sudo requis pour supprimer les règles udev : sudo rm $(UDEV_RULES)/50-xencelabs.rules"; }; \
	if [ "$$(id -u)" -eq 0 ]; then udevadm control --reload-rules 2>/dev/null; udevadm trigger 2>/dev/null; fi; \
	echo "Désinstallation terminée."
