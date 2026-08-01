import { readdirSync, statSync } from 'node:fs'

const MAX_KB = 1500 // presupuesto total JS; bajar cuando se haga code-split por rutas
const js = readdirSync('dist/assets').filter((f) => f.endsWith('.js'))
const total = js.reduce((s, f) => s + statSync(`dist/assets/${f}`).size, 0) / 1024
if (total > MAX_KB) {
  console.error(`Bundle ${total.toFixed(0)} KB > presupuesto ${MAX_KB} KB`)
  process.exit(1)
}
console.log(`Bundle OK: ${total.toFixed(0)} KB / ${MAX_KB} KB`)
