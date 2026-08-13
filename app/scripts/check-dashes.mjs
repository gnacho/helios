// Falla si aparecen em dash (U+2014) o en dash (U+2013) en prosa visible de la
// app: index.html, manifest PWA y las traducciones i18n (regresión #55/#86).
import { execSync } from 'node:child_process'

const TARGETS = ['index.html', 'public/manifest.webmanifest', 'src/i18n']
const out = execSync(`grep -rnP '[\\x{2013}\\x{2014}]' ${TARGETS.join(' ')} || true`).toString()
const bad = out.split('\n').filter((l) => l)
if (bad.length) {
  console.error('Em/en dashes (—/–) en texto visible de la app:\n' + bad.join('\n'))
  process.exit(1)
}
console.log('check-dashes OK: sin em/en dashes en index.html, manifest e i18n')
