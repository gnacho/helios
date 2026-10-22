# Tooling del UI templ (server-go/internal/web). Sin node: templ CLI + Tailwind standalone.
TEMPL  ?= $(HOME)/go/bin/templ
TW     ?= tools/tailwindcss
TW_VER ?= v4.3.3
WEB    := server-go/internal/web

.PHONY: web-templ web-css i18n-sync web-assets web-check

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

# Copia los catálogos i18n desde la app React (fuente de verdad hasta el swap).
i18n-sync:
	for l in es en zh-CN; do cp app/src/i18n/locales/$$l/translation.json $(WEB)/locales/$$l/translation.json; done

web-assets: web-templ web-css

# Verificación de CI: los artefactos generados deben estar al día en git.
web-check: web-assets i18n-sync
	@git diff --exit-code -- $(WEB) app/src/i18n > /dev/null || \
		(echo "ERROR: artefactos web desactualizados; ejecuta 'make web-assets && make i18n-sync' y commitea" && git diff --stat -- $(WEB) app/src/i18n && exit 1)
