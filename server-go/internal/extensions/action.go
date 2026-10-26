package extensions

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Errores de validación de una acción de cargador. El transporte decide el
// código HTTP (400 aquí, 502 si falla HA).
var (
	ErrChargerNoSwitch = errors.New("el cargador no tiene switch configurado")
	ErrChargerNoAmp    = errors.New("el cargador no tiene entidad de corriente configurada")
	ErrChargerAmps     = errors.New("amperaje fuera de rango (6-32 A)")
	ErrChargerDelay    = errors.New("hora de inicio fuera de rango (0-24 h)")
	ErrChargerDuration = errors.New("duración fuera de rango (1-12 h)")
	ErrChargerAction   = errors.New("acción no soportada")
	// Programación por software (cargadores sin dps de scheduling).
	ErrChargerNoSchedule    = errors.New("el cargador no tiene programación configurada")
	ErrChargerScheduleTime  = errors.New("hora de inicio no válida (HH:MM)")
	ErrChargerScheduleHours = errors.New("duración fuera de rango (1-12 h)")
)

var (
	switchIDRe = regexp.MustCompile(`^switch\.[a-z0-9_]+$`)
	ampIDRe    = regexp.MustCompile(`^(number|input_select|select)\.[a-z0-9_]+$`)
	schedIDRe  = regexp.MustCompile(`^(number|input_number|input_select|select)\.[a-z0-9_]+$`)
	dtIDRe     = regexp.MustCompile(`^input_datetime\.[a-z0-9_]+$`)
	numIDRe    = regexp.MustCompile(`^input_number\.[a-z0-9_]+$`)
	boolIDRe   = regexp.MustCompile(`^input_boolean\.[a-z0-9_]+$`)
	timerIDRe  = regexp.MustCompile(`^timer\.[a-z0-9_]+$`)
)

// scheduleWindow resolves a schedule from a wall clock time (HH:MM, today) and
// a duration in hours: the end is always start + duration. If the start already
// passed but the end is still ahead, the charge starts now and only the
// remaining time is applied. If the whole window passed, it moves to tomorrow.
func scheduleWindow(at string, hours float64, now time.Time) (time.Time, time.Time, error) {
	t, err := time.ParseInLocation("15:04", at, time.Local)
	if err != nil {
		return time.Time{}, time.Time{}, ErrChargerScheduleTime
	}
	if hours < 1 || hours > 12 {
		return time.Time{}, time.Time{}, ErrChargerScheduleHours
	}
	start := time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), 0, 0, time.Local)
	end := start.Add(time.Duration(hours * float64(time.Hour)))
	if !end.After(now) {
		start = start.AddDate(0, 0, 1)
		end = end.AddDate(0, 0, 1)
	}
	return start, end, nil
}

// setScheduleValue writes a number/input_number helper or a select helper,
// which are the two kinds of entities used for charger scheduling.
func setScheduleValue(extHA HA, id string, v float64) error {
	domain := id[:strings.Index(id, ".")]
	if domain == "number" || domain == "input_number" {
		return extHA.CallService(domain, "set_value", map[string]any{
			"entity_id": id, "value": v,
		})
	}
	return extHA.CallService(domain, "select_option", map[string]any{
		"entity_id": id, "option": strconv.FormatFloat(v, 'f', -1, 64),
	})
}

// ApplySchedule arms a one-shot schedule on a charger without hardware
// scheduling DPs (the wall): it writes the start time and duration helpers and
// turns the arm boolean on, so the HA automation fires at that time and stops
// the charge when the timer finishes.
func ApplySchedule(extHA HA, charger Charger, at string, hours float64) error {
	if extHA == nil {
		return errors.New("HAOS no conectado")
	}
	if !dtIDRe.MatchString(charger.ScheduleStartID) ||
		!numIDRe.MatchString(charger.ScheduleDurationID) ||
		!boolIDRe.MatchString(charger.ScheduleArmID) {
		return ErrChargerNoSchedule
	}
	now := time.Now()
	start, end, err := scheduleWindow(at, hours, now)
	if err != nil {
		return err
	}
	if err := extHA.CallService("input_datetime", "set_datetime", map[string]any{
		"entity_id": charger.ScheduleStartID, "time": start.Format("15:04:05"),
	}); err != nil {
		return err
	}
	if err := extHA.CallService("input_number", "set_value", map[string]any{
		"entity_id": charger.ScheduleDurationID, "value": math.Round(hours),
	}); err != nil {
		return err
	}
	if start.After(now) {
		return extHA.CallService("input_boolean", "turn_on", map[string]any{
			"entity_id": charger.ScheduleArmID,
		})
	}
	// La hora de inicio ya paso: se empieza ahora y el timer cubre solo lo que
	// queda hasta inicio + duracion.
	if !switchIDRe.MatchString(charger.SwitchID) {
		return ErrChargerNoSwitch
	}
	if !timerIDRe.MatchString(charger.ScheduleTimerID) {
		return ErrChargerNoSchedule
	}
	if err := extHA.CallService("input_boolean", "turn_off", map[string]any{
		"entity_id": charger.ScheduleArmID,
	}); err != nil {
		return err
	}
	if err := extHA.CallService("switch", "turn_on", map[string]any{
		"entity_id": charger.SwitchID,
	}); err != nil {
		return err
	}
	return extHA.CallService("timer", "start", map[string]any{
		"entity_id": charger.ScheduleTimerID, "duration": int(end.Sub(now).Seconds()),
	})
}

// ClearSchedule disarms a pending schedule and cancels its running timer.
func ClearSchedule(extHA HA, charger Charger) error {
	if extHA == nil {
		return errors.New("HAOS no conectado")
	}
	if !boolIDRe.MatchString(charger.ScheduleArmID) {
		return ErrChargerNoSchedule
	}
	if err := extHA.CallService("input_boolean", "turn_off", map[string]any{
		"entity_id": charger.ScheduleArmID,
	}); err != nil {
		return err
	}
	if timerIDRe.MatchString(charger.ScheduleTimerID) {
		return extHA.CallService("timer", "cancel", map[string]any{
			"entity_id": charger.ScheduleTimerID,
		})
	}
	return nil
}

// ApplyTimed pushes both scheduling values of a charger that has hardware
// delay/duration entities (portable F-M24 dps 118/119 via helpers): the delay
// in hours from now and the duration in hours.
func ApplyTimed(extHA HA, charger Charger, at string, hours float64) error {
	if extHA == nil {
		return errors.New("HAOS no conectado")
	}
	if !schedIDRe.MatchString(charger.DelayID) || !schedIDRe.MatchString(charger.DurationID) {
		return ErrChargerNoAmp
	}
	now := time.Now()
	start, end, err := scheduleWindow(at, hours, now)
	if err != nil {
		return err
	}
	effective := start
	if effective.Before(now) {
		effective = now
	}
	delayHours := math.Round(effective.Sub(now).Hours()*2) / 2
	durationHours := math.Round(end.Sub(effective).Hours()*2) / 2
	if delayHours < 0 || delayHours > 24 {
		return ErrChargerDelay
	}
	if durationHours < 0.5 || durationHours > 12 {
		return ErrChargerDuration
	}
	if err := setScheduleValue(extHA, charger.DelayID, delayHours); err != nil {
		return err
	}
	return setScheduleValue(extHA, charger.DurationID, durationHours)
}

// ApplyAction runs one charger action (on, off, amp) against Home Assistant.
// It is the single implementation shared by the JSON API and the templ UI, so
// both validate and call the same way. Returns the amps applied by "amp".
func ApplyAction(extHA HA, charger Charger, action string, amps *float64) (float64, error) {
	if extHA == nil {
		return 0, errors.New("HAOS no conectado")
	}
	switch action {
	case "on", "off":
		if !switchIDRe.MatchString(charger.SwitchID) {
			return 0, ErrChargerNoSwitch
		}
		service := "turn_off"
		if action == "on" {
			service = "turn_on"
		}
		if err := extHA.CallService("switch", service, map[string]any{"entity_id": charger.SwitchID}); err != nil {
			return 0, err
		}
		return 0, nil

	case "amp":
		if amps == nil {
			return 0, errors.New("amperaje requerido")
		}
		value := math.Round(*amps*10) / 10
		if value < 6 || value > 32 {
			return 0, ErrChargerAmps
		}
		if !ampIDRe.MatchString(charger.AmpID) {
			return 0, ErrChargerNoAmp
		}
		domain := charger.AmpID[:strings.Index(charger.AmpID, ".")]
		var err error
		if domain == "number" {
			err = extHA.CallService("number", "set_value", map[string]any{
				"entity_id": charger.AmpID, "value": value,
			})
		} else {
			err = extHA.CallService(domain, "select_option", map[string]any{
				"entity_id": charger.AmpID, "option": strconv.FormatFloat(value, 'f', -1, 64),
			})
		}
		if err != nil {
			return 0, err
		}
		return value, nil

	case "delay", "duration":
		if amps == nil {
			return 0, errors.New("valor requerido")
		}
		value := math.Round(*amps*2) / 2
		target := charger.DelayID
		if action == "duration" {
			value = math.Round(*amps)
			if value < 1 || value > 12 {
				return 0, ErrChargerDuration
			}
			target = charger.DurationID
		} else if value < 0 || value > 24 {
			return 0, ErrChargerDelay
		}
		if !schedIDRe.MatchString(target) {
			return 0, ErrChargerNoAmp
		}
		if err := setScheduleValue(extHA, target, value); err != nil {
			return 0, err
		}
		return value, nil

	default:
		return 0, fmt.Errorf("%w: %s", ErrChargerAction, action)
	}
}
