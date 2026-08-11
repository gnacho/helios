import { useEffect, useMemo, useState } from 'react';
import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { Plus, Save, Trash2 } from 'lucide-react';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Button } from '@/components/ui/button';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Switch } from '@/components/ui/switch';
import type { InstallInfo, Topology, TopologyInverter } from '@/data/types';
import { apiPut, ApiError } from '@/data/api-client';
import { invalidateInstall } from '@/hooks/useInstall';
import { heliosToast } from '@/lib/toast';
import { cn } from '@/lib/utils';

const csvToArr = (s: string): string[] =>
  s.split(',').map((x) => x.trim()).filter(Boolean);

const arrToCsv = (a: string[]): string => a.join(', ');

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  install: InstallInfo | null;
}

/** Editor de topología de la instalación (issue #41): el admin configura desde
 *  Ajustes cuántos inversores hay, la batería, cómo se lee la red y qué
 *  entidades de HAOS alimentan el histórico. Guarda install_config.topology. */
export function TopologyEditor({ open, onOpenChange, install }: Props) {
  const { t } = useTranslation();
  const [draft, setDraft] = useState<Topology | null>(null);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (open && install) {
      setDraft(JSON.parse(JSON.stringify(install.topology)) as Topology);
      setError(null);
    }
  }, [open, install]);

  const kv = useMemo(
    () => ({
      v: draft,
      set: (patch: Partial<Topology>) => setDraft((d) => (d ? { ...d, ...patch } : d)),
      inv: (i: number, patch: Partial<TopologyInverter>) =>
        setDraft((d) =>
          d
            ? { ...d, inverters: d.inverters.map((inv, idx) => (idx === i ? { ...inv, ...patch } : inv)) }
            : d,
        ),
    }),
    [draft],
  );

  const save = async () => {
    if (!draft) return;
    setSaving(true);
    setError(null);
    try {
      const res = await apiPut<{ ok: boolean; restartNeeded?: boolean }>('/api/config', { topology: draft });
      if (res.restartNeeded) {
        heliosToast(t('ajustes.topology.savedRestart'), { tone: 'warning' });
      } else {
        heliosToast(t('ajustes.topology.saved'), { tone: 'success' });
      }
      invalidateInstall();
      onOpenChange(false);
    } catch (e) {
      setError(e instanceof ApiError ? e.message : t('ajustes.topology.saveError'));
    } finally {
      setSaving(false);
    }
  };

  if (!draft) return null;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[88vh] max-w-4xl overflow-y-auto sm:max-w-4xl">
        <DialogHeader>
          <DialogTitle>{t('ajustes.topology.title')}</DialogTitle>
          <DialogDescription>{t('ajustes.topology.desc')}</DialogDescription>
        </DialogHeader>

        <div className="flex flex-col gap-6">
          {/* ── Inversores ── */}
          <fieldset className="flex flex-col gap-3">
            <legend className="text-[13px] font-semibold text-app">{t('ajustes.topology.inverters')}</legend>
            {draft.inverters.map((inv, i) => (
              <div key={inv.key} className="rounded-xl border border-app bg-surface p-3">
                <div className="mb-2 flex items-center justify-between">
                  <p className="text-xs font-semibold text-muted">
                    {t('ajustes.topology.inverterLabel', { n: i + 1 })}
                  </p>
                  {draft.inverters.length > 1 && (
                    <button
                      type="button"
                      onClick={() =>
                        setDraft((d) => (d ? { ...d, inverters: d.inverters.filter((_, idx) => idx !== i) } : d))
                      }
                      className="flex h-7 items-center gap-1 rounded-md px-2 text-xs font-medium text-destructive transition-colors hover:bg-destructive/10"
                      aria-label={t('ajustes.topology.removeInverter')}
                    >
                      <Trash2 size={13} />
                      {t('ajustes.topology.remove')}
                    </button>
                  )}
                </div>
                <div className="grid gap-3 sm:grid-cols-3">
                  <Field label={t('ajustes.topology.name')}>
                    <Input value={inv.name} onChange={(e) => kv.inv(i, { name: e.target.value })} />
                  </Field>
                  <Field label={t('ajustes.topology.model')}>
                    <Input value={inv.model} onChange={(e) => kv.inv(i, { model: e.target.value })} />
                  </Field>
                  <Field label={t('ajustes.topology.panels')}>
                    <Input
                      value={inv.panels}
                      placeholder="10 × 440 W"
                      onChange={(e) => kv.inv(i, { panels: e.target.value })}
                    />
                  </Field>
                  <Field label={t('ajustes.topology.kwp')}>
                    <Input
                      type="number"
                      step="0.1"
                      min={0}
                      value={inv.kwp || ''}
                      onChange={(e) => kv.inv(i, { kwp: Number(e.target.value) || 0 })}
                    />
                  </Field>
                  <Field label={t('ajustes.topology.powerId')}>
                    <Input
                      value={inv.powerId}
                      placeholder="sensor.inverter_power"
                      className="font-mono text-xs"
                      onChange={(e) => kv.inv(i, { powerId: e.target.value })}
                    />
                  </Field>
                  <div className="grid grid-cols-2 gap-3">
                    <Field label={t('ajustes.topology.powerUnit')}>
                      <Select
                        value={inv.powerUnit}
                        onValueChange={(v) => kv.inv(i, { powerUnit: v as 'kW' | 'W' })}
                      >
                        <SelectTrigger>
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="kW">kW</SelectItem>
                          <SelectItem value="W">W</SelectItem>
                        </SelectContent>
                      </Select>
                    </Field>
                    <Field label={t('ajustes.topology.energyCap')}>
                      <Input
                        type="number"
                        min={0}
                        value={inv.energyCap ?? ''}
                        onChange={(e) => kv.inv(i, { energyCap: Number(e.target.value) || 0 })}
                      />
                    </Field>
                  </div>
                  <Field label={t('ajustes.topology.energyId')}>
                    <Input
                      value={inv.energyId}
                      placeholder="sensor.inverter_energy_today"
                      className="font-mono text-xs"
                      onChange={(e) => kv.inv(i, { energyId: e.target.value })}
                    />
                  </Field>
                  <div className="grid grid-cols-2 gap-3">
                    <Field label={t('ajustes.topology.energyAcc')}>
                      <Select
                        value={inv.energyAcc}
                        onValueChange={(v) => kv.inv(i, { energyAcc: v as 'sum' | 'state' })}
                      >
                        <SelectTrigger>
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="state">{t('ajustes.topology.accState')}</SelectItem>
                          <SelectItem value="sum">{t('ajustes.topology.accSum')}</SelectItem>
                        </SelectContent>
                      </Select>
                    </Field>
                    <div className="flex items-end pb-1">
                      <label className="flex items-center gap-2 text-xs font-medium text-muted">
                        <Switch
                          checked={inv.hasBattery}
                          onCheckedChange={(v) => kv.inv(i, { hasBattery: v })}
                        />
                        {t('ajustes.topology.hasBattery')}
                      </label>
                    </div>
                  </div>
                  <Field label={t('ajustes.topology.deepIds')} className="sm:col-span-3">
                    <Input
                      value={arrToCsv(inv.deepIds)}
                      placeholder="sensor.inverter_energy_total"
                      className="font-mono text-xs"
                      onChange={(e) => kv.inv(i, { deepIds: csvToArr(e.target.value) })}
                    />
                  </Field>
                  {inv.hasBattery && (
                    <Field label={t('ajustes.topology.batteryKwh')}>
                      <Input
                        type="number"
                        step="0.1"
                        min={0}
                        value={inv.batteryKwh || ''}
                        onChange={(e) => kv.inv(i, { batteryKwh: Number(e.target.value) || 0 })}
                      />
                    </Field>
                  )}
                </div>
              </div>
            ))}
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="self-start"
              onClick={() =>
                setDraft((d) => {
                  if (!d) return d;
                  const n = d.inverters.length;
                  return {
                    ...d,
                    inverters: [
                      ...d.inverters,
                      {
                        key: `inv${n + 1}`,
                        name: t('ajustes.topology.inverterNew', { n: n + 1 }),
                        model: '',
                        kwp: 0,
                        panels: '',
                        tempC: 0,
                        hasBattery: false,
                        batteryKwh: 0,
                        powerId: '',
                        powerUnit: 'kW',
                        energyId: '',
                        energyAcc: 'state',
                        energyCap: 100,
                        deepIds: [],
                        glitchOffsets: {},
                      },
                    ],
                  };
                })
              }
            >
              <Plus size={14} />
              {t('ajustes.topology.addInverter')}
            </Button>
          </fieldset>

          {/* ── Batería ── */}
          <fieldset className="flex flex-col gap-3">
            <legend className="text-[13px] font-semibold text-app">{t('ajustes.topology.battery')}</legend>
            <div className="rounded-xl border border-app bg-surface p-3">
              <div className="grid gap-3 sm:grid-cols-3">
                <div className="flex items-center gap-2 sm:col-span-3">
                  <Switch
                    checked={draft.battery.enabled}
                    onCheckedChange={(v) => kv.set({ battery: { ...draft.battery, enabled: v } })}
                  />
                  <span className="text-xs font-medium text-muted">{t('ajustes.topology.enabled')}</span>
                </div>
                <Field label={t('ajustes.topology.powerId')}>
                  <Input
                    value={draft.battery.powerId}
                    placeholder="sensor.battery_power"
                    className="font-mono text-xs"
                    onChange={(e) => kv.set({ battery: { ...draft.battery, powerId: e.target.value } })}
                  />
                </Field>
                <Field label={t('ajustes.topology.capacityKwh')}>
                  <Input
                    type="number"
                    step="0.1"
                    min={0}
                    value={draft.battery.capacityKwh || ''}
                    onChange={(e) => kv.set({ battery: { ...draft.battery, capacityKwh: Number(e.target.value) || 0 } })}
                  />
                </Field>
                <Field label={t('ajustes.topology.stateId')}>
                  <Input
                    value={draft.battery.stateId}
                    placeholder="sensor.battery_state"
                    className="font-mono text-xs"
                    onChange={(e) => kv.set({ battery: { ...draft.battery, stateId: e.target.value } })}
                  />
                </Field>
                <Field label={t('ajustes.topology.socId')}>
                  <Input
                    value={draft.battery.socId}
                    placeholder="sensor.battery_soc"
                    className="font-mono text-xs"
                    onChange={(e) => kv.set({ battery: { ...draft.battery, socId: e.target.value } })}
                  />
                </Field>
                <Field label={t('ajustes.topology.chargingStates')}>
                  <Input
                    value={arrToCsv(draft.battery.chargingStates)}
                    placeholder="Cargando, charging"
                    className="font-mono text-xs"
                    onChange={(e) =>
                      kv.set({ battery: { ...draft.battery, chargingStates: csvToArr(e.target.value) } })
                    }
                  />
                </Field>
                <Field label={t('ajustes.topology.dischargingStates')}>
                  <Input
                    value={arrToCsv(draft.battery.dischargingStates)}
                    placeholder="Descargando, discharging"
                    className="font-mono text-xs"
                    onChange={(e) =>
                      kv.set({ battery: { ...draft.battery, dischargingStates: csvToArr(e.target.value) } })
                    }
                  />
                </Field>
              </div>
            </div>
          </fieldset>

          {/* ── Red ── */}
          <fieldset className="flex flex-col gap-3">
            <legend className="text-[13px] font-semibold text-app">{t('ajustes.topology.grid')}</legend>
            <div className="rounded-xl border border-app bg-surface p-3">
              <div className="grid gap-3 sm:grid-cols-3">
                <Field label={t('ajustes.topology.gridMode')}>
                  <Select value={draft.grid.mode} onValueChange={(v) => kv.set({ grid: { ...draft.grid, mode: v as 'attrs' | 'sensor' } })}>
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="attrs">{t('ajustes.topology.gridAttrs')}</SelectItem>
                      <SelectItem value="sensor">{t('ajustes.topology.gridSensor')}</SelectItem>
                    </SelectContent>
                  </Select>
                </Field>
                <Field label={t('ajustes.topology.statusAttrsId')} className="sm:col-span-2">
                  <Input
                    value={draft.statusAttrsId || ''}
                    placeholder="sensor.inverter_status"
                    className="font-mono text-xs"
                    onChange={(e) => kv.set({ statusAttrsId: e.target.value || null })}
                  />
                </Field>
                {draft.grid.mode === 'attrs' ? (
                  <Field label={t('ajustes.topology.attrsId')} className="sm:col-span-3">
                    <Input
                      value={draft.grid.attrsId || ''}
                      placeholder="sensor.grid_scraper"
                      className="font-mono text-xs"
                      onChange={(e) => kv.set({ grid: { ...draft.grid, attrsId: e.target.value || null } })}
                    />
                  </Field>
                ) : (
                  <>
                    <Field label={t('ajustes.topology.sensorId')}>
                      <Input
                        value={draft.grid.sensorId || ''}
                        placeholder="sensor.grid_power"
                        className="font-mono text-xs"
                        onChange={(e) => kv.set({ grid: { ...draft.grid, sensorId: e.target.value || null } })}
                      />
                    </Field>
                    <div className="grid grid-cols-2 gap-3 sm:col-span-2">
                      <Field label={t('ajustes.topology.importId')}>
                        <Input
                          value={draft.grid.importId || ''}
                          placeholder="sensor.grid_import_power"
                          className="font-mono text-xs"
                          onChange={(e) => kv.set({ grid: { ...draft.grid, importId: e.target.value || null } })}
                        />
                      </Field>
                      <Field label={t('ajustes.topology.exportId')}>
                        <Input
                          value={draft.grid.exportId || ''}
                          placeholder="sensor.grid_export_power"
                          className="font-mono text-xs"
                          onChange={(e) => kv.set({ grid: { ...draft.grid, exportId: e.target.value || null } })}
                        />
                      </Field>
                    </div>
                  </>
                )}
              </div>
            </div>
          </fieldset>

          {/* ── Consumo ── */}
          <fieldset className="flex flex-col gap-3">
            <legend className="text-[13px] font-semibold text-app">{t('ajustes.topology.consumption')}</legend>
            <div className="rounded-xl border border-app bg-surface p-3">
              <div className="grid gap-3 sm:grid-cols-3">
                <Field label={t('ajustes.topology.powerIds')} className="sm:col-span-3">
                  <Input
                    value={arrToCsv(draft.consumption.powerIds)}
                    placeholder="sensor.house_power, sensor.garage_power"
                    className="font-mono text-xs"
                    onChange={(e) => kv.set({ consumption: { ...draft.consumption, powerIds: csvToArr(e.target.value) } })}
                  />
                </Field>
                <Field label={t('ajustes.topology.energyIds')} className="sm:col-span-3">
                  <Input
                    value={arrToCsv(draft.consumption.energyIds)}
                    placeholder="sensor.house_energy"
                    className="font-mono text-xs"
                    onChange={(e) => kv.set({ consumption: { ...draft.consumption, energyIds: csvToArr(e.target.value) } })}
                  />
                </Field>
                <Field label={t('ajustes.topology.respaldoId')}>
                  <Input
                    value={draft.consumption.respaldoId || ''}
                    placeholder="sensor.backup_power"
                    className="font-mono text-xs"
                    onChange={(e) => kv.set({ consumption: { ...draft.consumption, respaldoId: e.target.value || null } })}
                  />
                </Field>
                <Field label={t('ajustes.topology.noRespaldadaId')} className="sm:col-span-2">
                  <Input
                    value={draft.consumption.noRespaldadaId || ''}
                    placeholder="sensor.non_backup_power"
                    className="font-mono text-xs"
                    onChange={(e) => kv.set({ consumption: { ...draft.consumption, noRespaldadaId: e.target.value || null } })}
                  />
                </Field>
              </div>
            </div>
          </fieldset>

          {/* ── Energía (histórico) ── */}
          <fieldset className="flex flex-col gap-3">
            <legend className="text-[13px] font-semibold text-app">{t('ajustes.topology.energy')}</legend>
            <div className="rounded-xl border border-app bg-surface p-3">
              <div className="grid gap-3 sm:grid-cols-3">
                <Field label={t('ajustes.topology.gridImportId')}>
                  <Input
                    value={draft.energy.gridImportId}
                    placeholder="sensor.grid_import_energy"
                    className="font-mono text-xs"
                    onChange={(e) => kv.set({ energy: { ...draft.energy, gridImportId: e.target.value } })}
                  />
                </Field>
                <Field label={t('ajustes.topology.gridExportId')}>
                  <Input
                    value={draft.energy.gridExportId}
                    placeholder="sensor.grid_export_energy"
                    className="font-mono text-xs"
                    onChange={(e) => kv.set({ energy: { ...draft.energy, gridExportId: e.target.value } })}
                  />
                </Field>
                <Field label={t('ajustes.topology.consumptionId')}>
                  <Input
                    value={draft.energy.consumptionId}
                    placeholder="sensor.house_energy_today"
                    className="font-mono text-xs"
                    onChange={(e) => kv.set({ energy: { ...draft.energy, consumptionId: e.target.value } })}
                  />
                </Field>
                <Field label={t('ajustes.topology.batChargeId')}>
                  <Input
                    value={draft.energy.batChargeId}
                    placeholder="sensor.battery_charge_energy"
                    className="font-mono text-xs"
                    onChange={(e) => kv.set({ energy: { ...draft.energy, batChargeId: e.target.value } })}
                  />
                </Field>
                <Field label={t('ajustes.topology.batDischargeId')} className="sm:col-span-2">
                  <Input
                    value={draft.energy.batDischargeId}
                    placeholder="sensor.battery_discharge_energy"
                    className="font-mono text-xs"
                    onChange={(e) => kv.set({ energy: { ...draft.energy, batDischargeId: e.target.value } })}
                  />
                </Field>
              </div>
            </div>
          </fieldset>

          {/* ── Sol y tiempo ── */}
          <fieldset className="flex flex-col gap-3">
            <legend className="text-[13px] font-semibold text-app">{t('ajustes.topology.environment')}</legend>
                        <div className="grid gap-3 rounded-xl border border-app bg-surface p-3 sm:grid-cols-3">
              <Field label={t('ajustes.topology.sun')}>
                <Input
                  value={draft.sun}
                  placeholder="sun.sun"
                  className="font-mono text-xs"
                  onChange={(e) => kv.set({ sun: e.target.value })}
                />
              </Field>
              <Field label={t('ajustes.topology.weather')}>


                <Input
                  value={draft.weather}
                  placeholder="weather.forecast"
                  className="font-mono text-xs"
                  onChange={(e) => kv.set({ weather: e.target.value })}
                />
              </Field>
              <Field label={t('ajustes.topology.weatherTemp')}>
                <Input
                  value={draft.weatherTemp}
                  placeholder="sensor.outdoor_temperature"
                  className="font-mono text-xs"
                  onChange={(e) => kv.set({ weatherTemp: e.target.value })}
                />
              </Field>
            </div>
          </fieldset>
        </div>

        {error && (
          <p role="alert" className="rounded-lg border border-destructive/30 bg-destructive/10 px-3 py-2 text-xs font-medium text-destructive">
            {error}
          </p>
        )}

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={saving}>
            {t('common.cancel')}
          </Button>
          <Button onClick={save} disabled={saving}>
            {saving ? (
              t('ajustes.topology.saving')
            ) : (
              <>
                <Save size={15} />
                {t('ajustes.topology.save')}
              </>
            )}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function Field({ label, className, children }: { label: string; className?: string; children: ReactNode }) {
  return (
    <div className={cn('flex flex-col gap-1.5', className)}>
      <Label className="text-xs font-medium text-muted">{label}</Label>
      {children}
    </div>
  );
}
