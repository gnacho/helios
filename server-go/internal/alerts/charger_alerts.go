package alerts

import (
	"fmt"
	"time"

	"helios-server/internal/extensions"
	"helios-server/internal/ha"
	"helios-server/internal/push"
)

// ChgCargadorEstado recuerda por cargador si hay una sesión programada en
// marcha, para que los avisos de inicio/fin se disparen una vez por
// transición y no en cada tick.
type ChgCargadorEstado struct {
	Cargando   bool
	Programada bool
}

// haAdapter eleva el HA del motor (GetState + IsConnected) a la interfaz de
// extensions: ComputeOneChargerLive solo lee estados, el resto queda sin uso.
type haAdapter struct{ HA }

func (haAdapter) StatisticsDuringPeriod(string, string, []string, string, []string) (ha.Stats, error) {
	return ha.Stats{}, nil
}

func (haAdapter) HistoryDuringPeriod(string, string, string) ([]ha.HistoryRow, error) {
	return nil, nil
}

func (haAdapter) CallService(string, string, map[string]any) error { return nil }

// tickChargers evalúa los cargadores con programación: push cuando empieza la
// carga programada (armada o con el timer HA en marcha) y cuando acaba. Una
// sesión que arranca sin programación (botón Encender) no avisa.
func (e *Engine) tickChargers() {
	if e.ChargersFn == nil {
		return
	}
	h := haAdapter{e.HA}
	for _, c := range e.ChargersFn() {
		if c.ScheduleArmID == "" && c.ScheduleTimerID == "" {
			continue
		}
		live := extensions.ComputeOneChargerLive(h, c)
		st := e.Estado.Chg[c.Key]
		if st == nil {
			st = &ChgCargadorEstado{}
			e.Estado.Chg[c.Key] = st
		}
		if live.Charging && !st.Cargando {
			st.Cargando = true
			// Programada: armada en el momento del arranque, o el timer HA
			// ya corre (ApplySchedule apaga el arm y arranca el timer cuando
			// la hora ya pasó; las automatizaciones de HA hacen lo mismo a la
			// hora planeada).
			st.Programada = live.Scheduled || live.TimerRemainingS != nil
			if st.Programada {
				datos := push.Datos{"nombre": c.Name}
				if live.TimerRemainingS != nil {
					fin := time.Now().Add(time.Duration(*live.TimerRemainingS) * time.Second).Format("15:04")
					datos["fin"] = fin
				} else if live.ScheduleHours != nil {
					datos["duracion"] = fmt.Sprintf("%g", *live.ScheduleHours)
				}
				e.disparar("carga_inicio", datos, push.Options{Severity: "normal"})
			}
		} else if !live.Charging && st.Cargando {
			st.Cargando = false
			if st.Programada {
				st.Programada = false
				e.disparar("carga_fin", push.Datos{"nombre": c.Name, "fin": time.Now().Format("15:04")}, push.Options{Severity: "normal"})
			}
		}
	}
}
