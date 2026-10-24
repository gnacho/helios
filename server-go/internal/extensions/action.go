package extensions

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// Errores de validación de una acción de cargador. El transporte decide el
// código HTTP (400 aquí, 502 si falla HA).
var (
	ErrChargerNoSwitch = errors.New("el cargador no tiene switch configurado")
	ErrChargerNoAmp    = errors.New("el cargador no tiene entidad de corriente configurada")
	ErrChargerAmps     = errors.New("amperaje fuera de rango (6-32 A)")
	ErrChargerAction   = errors.New("acción no soportada")
)

var (
	switchIDRe = regexp.MustCompile(`^switch\.[a-z0-9_]+$`)
	ampIDRe    = regexp.MustCompile(`^(number|input_select|select)\.[a-z0-9_]+$`)
)

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

	default:
		return 0, fmt.Errorf("%w: %s", ErrChargerAction, action)
	}
}
