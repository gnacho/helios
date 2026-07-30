import { useCallback, useEffect, useState } from 'react';

/**
 * Ajustes de la app persistidos en localStorage (mock de iteración 1).
 * En la fase 2 la conexión HAOS y los precios vendrán del backend local.
 */
export interface EnergySettings {
  /** Precio de compra de red (€/kWh evitado). */
  priceImport: number;
  /** Compensación por vertido (€/kWh). */
  priceExport: number;
  /** Factor CO₂ evitado (kg/kWh producido). */
  co2Factor: number;
  /** Nombre de la instalación. */
  installName: string;
  /** Ubicación elegida (población). */
  location: string;
  /** Coordenadas de la ubicación elegida (null = usar las de Home Assistant). */
  locationLat: number | null;
  locationLon: number | null;
}

export const DEFAULT_SETTINGS: EnergySettings = {
  priceImport: 0.15,
  priceExport: 0.08,
  co2Factor: 0.25,
  installName: 'Casa',
  location: '',
  locationLat: null,
  locationLon: null,
};

const STORAGE_KEY = 'helios-settings';
const CHANGE_EVENT = 'helios-settings-changed';

export function loadSettings(): EnergySettings {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (raw) return { ...DEFAULT_SETTINGS, ...(JSON.parse(raw) as Partial<EnergySettings>) };
  } catch {
    /* sin localStorage */
  }
  return DEFAULT_SETTINGS;
}

export function saveSettings(next: EnergySettings) {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(next));
  } catch {
    /* sin localStorage */
  }
}

/**
 * Hook de ajustes: lee de localStorage al montar y persiste cada cambio.
 * Las instancias concurrentes (varias secciones de Ajustes) se mantienen
 * sincronizadas mediante un evento propio; `update` fusiona siempre sobre el
 * último valor persistido para no pisar campos de otras instancias.
 */
export function useEnergySettings(): [EnergySettings, (patch: Partial<EnergySettings>) => void] {
  const [settings, setSettings] = useState<EnergySettings>(loadSettings);

  useEffect(() => {
    const sync = () => setSettings(loadSettings());
    window.addEventListener(CHANGE_EVENT, sync);
    window.addEventListener('storage', sync);
    return () => {
      window.removeEventListener(CHANGE_EVENT, sync);
      window.removeEventListener('storage', sync);
    };
  }, []);

  const update = useCallback((patch: Partial<EnergySettings>) => {
    const next = { ...loadSettings(), ...patch };
    saveSettings(next);
    setSettings(next);
    window.dispatchEvent(new Event(CHANGE_EVENT));
  }, []);

  return [settings, update];
}
