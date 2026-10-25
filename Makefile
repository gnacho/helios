# Tooling del UI templ (server-go/internal/web). Sin node: templ CLI + Tailwind standalone.
TEMPL  ?= $(HOME)/go/bin/templ
TW     ?= tools/tailwindcss
TW_VER ?= v4.3.3
WEB    := server-go/internal/web

.PHONY: web-templ web-css web-assets web-check

# Descarga el binario standalone de Tailwind (sin node) si no existe.
tools/tailwindcss:
	mkdir -p tools
	curl -sL -o $(TW) https://github.com/tailwindlabs/tailwindcss/releases/download/$(TW_VER)/tailwindcss-linux-x64
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
web-check: web-assets
	@git diff --exit-code -- $(WEB) > /dev/null || \
		(echo "ERROR: artefactos web desactualizados; ejecuta 'make web-assets' y commitea" && git diff --stat -- $(WEB) && exit 1)
