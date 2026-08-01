// Falla si un componente hardcodea colores hex fuera de los ficheros de tokens
import { execSync } from 'node:child_process'

const ALLOW = ['src/index.css', 'src/lib/colors.ts', 'tailwind.config']
const out = execSync(
  `grep -rnE '#[0-9a-fA-F]{3}([0-9a-fA-F]{3})?([0-9a-fA-F]{2})?\\b' src --include='*.ts' --include='*.tsx' || true`,
).toString()
const bad = out.split('\n').filter((l) => l && !ALLOW.some((a) => l.startsWith(a)))
if (bad.length) {
  console.error('Colores hex fuera de los tokens de diseño:\n' + bad.join('\n'))
  process.exit(1)
}
console.log('theme-audit OK: sin hex fuera de tokens')
