# Tooling del UI templ (server-go/internal/web). Sin node: templ CLI + Tailwind standalone.
TEMPL  ?= $(HOME)/go/bin/templ
TW        ?= tools/tailwindcss
TW_VER    ?= v4.3.3

# La matriz de release compila en amd64 y en arm64: el binario standalone debe
# coincidir con la arquitectura del runner, o no se puede ejecutar.
UNAME_M := $(shell uname -m)
ifeq ($(filter aarch64 arm64,$(UNAME_M)),)
TW_ARCH ?= x64
else
TW_ARCH ?= arm64
endif

TW_ASSET  ?= tailwindcss-linux-$(TW_ARCH)
TW_SHA256_x64   := dc61b3ac6b8c9ca874c0cc4c57b2409791a64c5540404ca5f5367360babc313a
TW_SHA256_arm64 := 55fd0b241214eff3de1e8ee4f22796662f2d2e7a49bcfca7477cfd0bac398195
TW_SHA256 ?= $(TW_SHA256_$(TW_ARCH))
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
