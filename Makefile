# Tooling del UI templ (server-go/internal/web). Sin node: templ CLI + Tailwind standalone.
TEMPL  ?= $(HOME)/go/bin/templ
TW        ?= tools/tailwindcss
TW_VER    ?= v4.3.3
TW_ASSET  ?= tailwindcss-linux-x64
TW_SHA256 ?= dc61b3ac6b8c9ca874c0cc4c57b2409791a64c5540404ca5f5367360babc313a
WEB       := server-go/internal/web

.PHONY: web-templ web-css web-assets web-check

# Descarga el binario standalone de Tailwind (sin node) si no existe.
# El digest está fijado en TW_SHA256 y se contrasta con el sha256sums.txt de la release.
tools/tailwindcss:
	mkdir -p tools
	curl -fsSL --max-time 120 --max-filesize 134217728 -o $(TW).tmp https://github.com/tailwindlabs/tailwindcss/releases/download/$(TW_VER)/$(TW_ASSET)
	echo "$(TW_SHA256)  $(TW).tmp" | sha256sum -c -
	mv -f $(TW).tmp $(TW)
	chmod +x $(TW)

# Genera el Go de las plantillas .templ.
web-templ:
	cd server-go && $(TEMPL) generate .

# Compila el CSS (Tailwind v4 standalone; escanea server-go vía source()).
web-css: $(TW)
	$(TW) -i $(WEB)/assets/css/app.css -o $(WEB)/assets/css/app.build.css

web-assets: web-templ web-css

# Binario con artefactos web SIEMPRE al día (evita CSS/templ stale embebidos).
build: web-assets
	cd server-go && go build -o $(or $(BIN),/tmp/opencode/helios-go-templ) ./cmd/server

# Verificación de CI: los artefactos generados deben estar al día en git.
# Comprueba cambios en ficheros rastreados Y artefactos nuevos sin rastrear:
# `git diff` por sí solo no ve un fichero generado que aún no está en el índice.
web-check: web-assets
	@untracked=$$(git ls-files --others --exclude-standard -- $(WEB)); \
	if [ -n "$$untracked" ]; then \
		echo "ERROR: artefactos web generados sin rastrear; añádelos con git add" >&2; \
		printf '%s\n' "$$untracked" >&2; \
		exit 1; \
	fi
	@git diff --exit-code -- $(WEB) > /dev/null || \
		(echo "ERROR: artefactos web desactualizados; ejecuta 'make web-assets' y commitea" && git diff --stat -- $(WEB) && exit 1)
