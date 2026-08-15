import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';

/**
 * Mini-mapa estático del coche (issue 100): grid de tiles OSM + pin en la
 * posición exacta, sin librerías (Leaflet pesaba 146 KB y el presupuesto de
 * bundle está en 1500). Click → OpenStreetMap con marcador (en móvil abre la
 * app de mapas). Tiles vía CSP img-src *.tile.openstreetmap.org.
 */

const ZOOM = 15;
const TILES_X = 4;
const TILES_Y = 3;

/** Tile x/y Web-Mercator de una coordenada (fracciones incluidas). */
function degToTile(lat: number, lon: number, z: number) {
  const n = 2 ** z
  const x = ((lon + 180) / 360) * n
  const rad = (lat * Math.PI) / 180
  const y = ((1 - Math.log(Math.tan(rad) + 1 / Math.cos(rad)) / Math.PI) / 2) * n
  return { x, y }
}

export default function CarMap({ lat, lon, label }: { lat: number; lon: number; label?: string }) {
  const { t } = useTranslation()

  const { tiles, pinX, pinY, osmUrl } = useMemo(() => {
    const { x, y } = degToTile(lat, lon, ZOOM)
    const tx = Math.floor(x)
    const ty = Math.floor(y)
    const grid: { url: string; col: number; row: number }[] = []
    const x0 = tx - Math.floor(TILES_X / 2)
    const y0 = ty - Math.floor(TILES_Y / 2)
    for (let row = 0; row < TILES_Y; row++) {
      for (let col = 0; col < TILES_X; col++) {
        grid.push({
          url: `https://tile.openstreetmap.org/${ZOOM}/${x0 + col}/${y0 + row}.png`,
          col,
          row,
        })
      }
    }
    // Pin: posición fraccionaria dentro de la grid (0..1)
    const pinX = (x - x0) / TILES_X
    const pinY = (y - y0) / TILES_Y
    const osmUrl = `https://www.openstreetmap.org/?mlat=${lat}&mlon=${lon}#map=${ZOOM + 2}/${lat}/${lon}`
    return { tiles: grid, pinX, pinY, osmUrl }
  }, [lat, lon])

  return (
    <a
      href={osmUrl}
      target="_blank"
      rel="noopener noreferrer"
      aria-label={t('coche.mapLabel')}
      title={label}
      className="group relative block h-56 w-full overflow-hidden rounded-xl border border-app bg-surface-2 sm:h-64"
      style={{ display: 'grid', gridTemplateColumns: `repeat(${TILES_X}, 1fr)` }}
    >
      {tiles.map((tile) => (
        <img key={tile.url} src={tile.url} alt="" loading="lazy" className="h-full w-full object-cover" draggable={false} />
      ))}
      {/* Pin en la posición exacta del coche */}
      <span
        aria-hidden="true"
        className="pointer-events-none absolute z-10 -translate-x-1/2 -translate-y-full transition-transform group-hover:scale-110"
        style={{ left: `${pinX * 100}%`, top: `${pinY * 100}%` }}
      >
        <span
          className="block h-5 w-5 rounded-full border-[3px] border-white shadow-md"
          style={{ background: 'rgb(var(--accent-rgb))' }}
        />
        <span
          className="mx-auto -mt-1 block h-3 w-1.5"
          style={{ background: 'rgb(var(--accent-rgb))', clipPath: 'polygon(0 0, 100% 0, 50% 100%)' }}
        />
      </span>
      <span className="pointer-events-none absolute bottom-1.5 right-1.5 rounded-md bg-app/80 px-1.5 py-0.5 text-[10px] font-medium text-muted backdrop-blur-sm">
        OpenStreetMap
      </span>
    </a>
  )
}
