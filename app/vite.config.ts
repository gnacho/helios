import path from "path"
import react from "@vitejs/plugin-react"
import tailwindcss from "@tailwindcss/vite"
import { defineConfig, type Plugin } from "vite"

// Tracker GoatCounter SOLO en el build de la demo pública (issue #112):
//   VITE_GC_COUNT=https://stats.helios.cloudless.club npm run build
// Los builds normales NO lo llevan: una instalación self-hosted nunca debe
// llamar a casa. Los hits se registran con prefijo /demo en el mismo site
// que la landing ("/" = landing, "/demo/..." = demo).
const gcCount = process.env.VITE_GC_COUNT?.replace(/\/$/, "")

function goatcounterPlugin(): Plugin {
  return {
    name: "helios-goatcounter",
    transformIndexHtml(html) {
      if (!gcCount) return html
      const snippet =
        `    <script>window.goatcounter={path:function(p){return '/demo'+p}}</script>\n` +
        `    <script async data-goatcounter="${gcCount}/count" src="${gcCount}/count.js"></script>\n  </head>`
      return html.replace("</head>", snippet)
    },
  }
}

// https://vite.dev/config/
export default defineConfig({
  base: './',
  plugins: [react(), tailwindcss(), goatcounterPlugin()],
  server: {
    port: 3000,
  },
  resolve: {
    alias: {
      "@": path.resolve(import.meta.dirname, "./src"),
    },
  },
  build: {
    sourcemap: true,
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (!id.includes('node_modules')) return;
          if (
            id.includes('/recharts') ||
            id.includes('/d3-') ||
            id.includes('/recharts-')
          ) {
            return 'recharts';
          }
          if (
            id.includes('/framer-motion') ||
            id.includes('/gsap') ||
            id.includes('/@gsap/')
          ) {
            return 'motion';
          }
        },
      },
    },
  },
});
