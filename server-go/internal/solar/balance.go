package solar

import (
	"math"

	"helios-server/internal/install"
)

// LivePower es la foto instantánea de potencia que alimenta el balance de la
// casa: producción + red - batería, la misma fórmula que Home Assistant expone
// como sensor.consumo_total_vivienda. Es la única vía para el consumo en vivo y
// para el reparto de fotovoltaica del cargador.
type LivePower struct {
	Inverters      []LiveInverter
	Production     float64
	Grid           float64
	BatteryPower   float64
	GridConfigured bool
	GridDown       bool
}

// HouseConsumption es el consumo de la casa derivado del balance.
type HouseConsumption struct {
	Kw       float64
	FromGrid bool // viene del balance y no de las pinzas
	OK       bool // false si hay fuente de red configurada y caída
}

// ReadLivePower lee inversores, red y batería de Home Assistant con la topología
// de la instalación.
func ReadLivePower(t install.Topology, h HAReader) LivePower {
	p := LivePower{Inverters: make([]LiveInverter, 0, len(t.Inverters))}

	for _, inv := range t.Inverters {
		kw := inverterPowerKw(h, inv)
		if kw < 0.02 {
			kw = 0
		}
		p.Production += kw
		p.Inverters = append(p.Inverters, LiveInverter{Key: inv.Key, Name: inv.Name, Kw: Round3(kw)})
	}

	if t.Battery.Enabled {
		mag := entityNum(h, t.Battery.PowerID, 1)
		p.BatteryPower = batteryPowerKw(t.Battery, h, mag)
	}

	p.Grid, p.GridDown = readGrid(t.Grid, h)
	p.GridConfigured = (t.Grid.Mode == "attrs" && t.Grid.AttrsID != nil && *t.Grid.AttrsID != "") ||
		(t.Grid.SensorID != nil && *t.Grid.SensorID != "") ||
		t.Grid.ImportID != nil || t.Grid.ExportID != nil

	// De noche no puede haber export solar.
	if !p.GridDown && p.Production < 0.02 && p.Grid < -0.03 {
		if sun, _ := h.GetState(t.Sun); attrNum(sun.Attributes, "elevation") < 0 {
			p.Grid = -p.Grid
		}
	}
	return p
}

// readGrid devuelve la red con signo (positivo importación, negativo vertido) y
// si la fuente configurada está caída.
func readGrid(g install.GridConfig, h HAReader) (grid float64, down bool) {
	if g.Mode == "attrs" && g.AttrsID != nil && *g.AttrsID != "" {
		st, _ := h.GetState(*g.AttrsID)
		gridMag := math.Abs(attrNum(st.Attributes, "currentGridPower"))
		dir := attrStr(st.Attributes, "gridDirection")
		if dir == "" {
			dir = "none"
		}
		switch dir {
		case "import":
			grid = gridMag
		case "export":
			grid = -gridMag
		default:
			grid = 0
		}
	} else if g.SensorID != nil && *g.SensorID != "" {
		grid = entityNum(h, *g.SensorID, 1)
	} else {
		imp := 0.0
		if g.ImportID != nil {
			imp = entityNum(h, *g.ImportID, 1)
		}
		exp := 0.0
		if g.ExportID != nil {
			exp = entityNum(h, *g.ExportID, 1)
		}
		if imp > 0 {
			grid = imp
		} else {
			grid = -exp
		}
	}

	if !h.IsConnected() {
		return grid, false
	}
	if g.Mode == "attrs" && g.AttrsID != nil {
		st, ok := h.GetState(*g.AttrsID)
		return grid, isDown(st, ok)
	}
	if g.SensorID != nil {
		st, ok := h.GetState(*g.SensorID)
		return grid, isDown(st, ok)
	}
	if g.ImportID != nil {
		if st, ok := h.GetState(*g.ImportID); isDown(st, ok) {
			return grid, true
		}
	}
	if g.ExportID != nil {
		if st, ok := h.GetState(*g.ExportID); isDown(st, ok) {
			return grid, true
		}
	}
	return grid, false
}

// HouseConsumption convierte las lecturas en el consumo de la casa. metersKw es
// la suma de las pinzas y metersDown dice si alguna está caída, que solo importa
// cuando no hay fuente de red y las pinzas son la única opción. Con fuente de red
// configurada pero caída no hay balance y el consumo se declara no disponible en
// lugar de servir una cifra varias veces menor.
func (p LivePower) HouseConsumption(metersKw float64, metersDown bool) HouseConsumption {
	switch {
	case p.GridConfigured && !p.GridDown:
		kw := p.Production + p.Grid - p.BatteryPower
		if kw < 0 {
			kw = 0
		}
		return HouseConsumption{Kw: kw, FromGrid: true, OK: true}
	case p.GridConfigured:
		return HouseConsumption{Kw: metersKw, OK: false}
	case metersDown:
		return HouseConsumption{Kw: metersKw, OK: false}
	default:
		return HouseConsumption{Kw: metersKw, OK: true}
	}
}
