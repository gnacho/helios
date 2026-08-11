export interface PowerPoint {
  t: number;
  label: string;
  solis: number;
  fox: number;
  inverters?: Record<string, number>;
  production: number;
  consumption: number;
  batteryPower: number;
  soc: number;
  grid: number;
}

/** Valor de serie de un inversor por clave: p.inverters[key] o p[key] (solis/fox). */
export function seriesInvValue(p: PowerPoint, key: string): number {
  if (p.inverters && key in p.inverters) return p.inverters[key];
  return (p as unknown as Record<string, number>)[key] ?? 0;
}

export interface LiveAlert {
  id: string;
  severity: 'critical' | 'warning' | 'info';
  text: string;
}

export interface LiveInverter {
  key: string;
  name: string;
  kw: number;
}

export interface LivePower {
  production: number;
  consumption: number;
  batteryPower: number;
  soc: number;
  grid: number;
  solis: number;
  fox: number;
  inverters?: LiveInverter[];
  at: number;
  alerts?: LiveAlert[];
  batteryStatus?: string;
  sun?: {
    state: string;
    elevation: number;
    nextRising: string | null;
    nextSetting: string | null;
  };
  weather?: string;
  weatherTemp?: number;
  station?: string;
  inverterOnline?: boolean;
  ts?: string;
}

export interface DayKpis {
  productionKwh: number;
  consumptionKwh: number;
  gridImportKwh: number;
  gridExportKwh: number;
  batteryChargedKwh: number;
  batteryDischargedKwh: number;
  soc: number;
  autoconsumoPct: number;
  autosuficienciaPct: number;
  ahorroEur: number;
  co2EvitadoKg: number;
  peakProductionKw: number;
  peakAt: number;
  solisKwh?: number;
  foxKwh?: number;
  invertersKwh?: Record<string, number>;
}

/** Un inversor dentro de la topología (editable, issue #41). */
export interface TopologyInverter {
  key: string;
  name: string;
  model: string;
  kwp: number;
  panels: string;
  tempC: number;
  hasBattery: boolean;
  batteryKwh: number;
  powerId: string;
  powerUnit: 'kW' | 'W';
  energyId: string;
  energyAcc: 'sum' | 'state';
  energyCap: number;
  deepIds: string[];
  glitchOffsets: Record<string, number>;
}

/** Topología completa de la instalación: lo que se guarda como install_config.topology. */
export interface Topology {
  inverters: TopologyInverter[];
  battery: {
    enabled: boolean;
    powerId: string;
    stateId: string;
    socId: string;
    capacityKwh: number;
    chargingStates: string[];
    dischargingStates: string[];
  };
  grid: {
    mode: 'attrs' | 'sensor';
    attrsId: string | null;
    sensorId: string | null;
    importId: string | null;
    exportId: string | null;
  };
  statusAttrsId: string | null;
  consumption: {
    powerIds: string[];
    powerUnit: 'W' | 'kW';
    energyIds: string[];
    respaldoId: string | null;
    noRespaldadaId: string | null;
  };
  energy: {
    gridImportId: string;
    gridExportId: string;
    batChargeId: string;
    batDischargeId: string;
    consumptionId: string;
  };
  sun: string;
  weather: string;
  weatherTemp: string;
}

export interface InstallInfo {
  configured: boolean;
  topology: Topology;
  inverters: {
    key: string;
    name: string;
    model: string;
    kwp: number;
    panels: string;
    hasBattery: boolean;
    batteryKwh: number;
  }[];
  battery: {
    enabled: boolean;
    capacityKwh: number;
  };
  grid: {
    mode: 'attrs' | 'sensor';
  };
  entities: {
    role: string;
    entidad: string;
    key?: string;
    name?: string;
  }[];
}

export interface HistoryDay {
  date: Date;
  productionKwh: number;
  consumptionKwh: number;
  gridImportKwh: number;
  gridExportKwh: number;
  batteryChargedKwh?: number;
  batteryDischargedKwh?: number;
  autoconsumoPct: number;
}

export type ConnectionStatus = 'connected' | 'reconnecting' | 'demo';

export const STEP_MIN = 5;
export const LIVE_STALE_MS = 45_000;
export const DAY_POINTS = (24 * 60) / STEP_MIN;
export const SOLIS_KWP = 4.4;
export const FOX_KWP = 2.7;
export const SUNRISE_MIN = 6 * 60 + 50;
export const SUNSET_MIN = 21 * 60 + 40;
export const BATTERY_CAPACITY_KWH = 5.12;
export { BATTERY_RESERVE_PCT } from '@/lib/thresholds';
export const PRICE_IMPORT_EUR = 0.15;
export const PRICE_EXPORT_EUR = 0.08;
export const CO2_KG_PER_KWH = 0.25;
