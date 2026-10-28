package push

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/json"
	"errors"
	"log"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"

	"helios-server/internal/db"
)

type Lang string

const (
	LangES Lang = "es"
	LangEN Lang = "en"
	LangZH Lang = "zh"
)

type Datos map[string]any

type Options struct {
	Severity string
	URL      string
	TTL      int
}

// SendFn inyectable para tests.
type SendFn func(sub *webpush.Subscription, payload []byte, opts *webpush.Options) (int, error)

type Service struct {
	DB         *db.DB
	PublicKey  string
	PrivateKey string
	Subject    string
	configured bool
	sendFn     SendFn
}

func New(database *db.DB, publicKey, privateKey, subject string) *Service {
	s := &Service{DB: database, PublicKey: publicKey, PrivateKey: privateKey, Subject: subject}
	s.configured = publicKey != "" && privateKey != "" && subject != ""
	if !s.configured {
		log.Printf("[push] sin VAPID_PUBLIC_KEY/PRIVATE_KEY/SUBJECT: notificaciones push desactivadas")
	} else {
		log.Printf("[push] VAPID configurado: notificaciones push activas")
	}
	s.sendFn = defaultSendFn()
	return s
}

func (s *Service) SetSendFn(fn SendFn) {
	if fn == nil {
		s.sendFn = defaultSendFn()
		return
	}
	s.sendFn = fn
}

func (s *Service) IsConfigured() bool { return s.configured }

func (s *Service) PublicKeyOrEmpty() string {
	if !s.configured {
		return ""
	}
	return s.PublicKey
}

// TiposAlerta son los tipos conocidos (validación de preferencias).
var TiposAlerta = []string{
	"inversor_offline", "inversor_ok",
	"fox_offline", "fox_ok",
	"corte_red", "corte_red_ok",
	"bateria_baja", "bateria_llena",
	"resumen_diario",
}

var severidades = []string{"normal", "high", "critical"}

func severidadIdx(s string) int {
	for i, v := range severidades {
		if v == s {
			return i
		}
	}
	return 0
}

func IsValidTipo(t string) bool {
	for _, x := range TiposAlerta {
		if x == t {
			return true
		}
	}
	return false
}

// ---- Catálogo i18n (texto final compuesto en servidor) ----

type entrada struct {
	titulo string
	cuerpo func(Datos) string
}

func s(d Datos, k, def string) string {
	if v, ok := d[k].(string); ok && v != "" {
		return v
	}
	return def
}

func n(d Datos, k string) string {
	v, ok := d[k]
	if !ok {
		return ""
	}
	switch x := v.(type) {
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(x), 'f', -1, 64)
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case string:
		return x
	}
	return ""
}

var catalogo = map[Lang]map[string]entrada{
	LangES: {
		"inversor_offline": {titulo: "Inversor offline", cuerpo: func(d Datos) string { return "El inversor no responde" }},
		"inversor_ok":      {titulo: "Inversor recuperado", cuerpo: func(d Datos) string { return "El inversor vuelve a estar online" }},
		"fox_offline": {titulo: "Inversor offline", cuerpo: func(d Datos) string {
			return "El inversor " + s(d, "nombre", "secundario") + " no reporta datos (de día)"
		}},
		"fox_ok":        {titulo: "Inversor recuperado", cuerpo: func(d Datos) string { return "El inversor " + s(d, "nombre", "secundario") + " vuelve a reportar" }},
		"corte_red":     {titulo: "Corte de red", cuerpo: func(d Datos) string { return "Posible corte de red eléctrica: consumo desde batería/PV" }},
		"corte_red_ok":  {titulo: "Red recuperada", cuerpo: func(d Datos) string { return "La red eléctrica vuelve a estar presente" }},
		"bateria_baja":  {titulo: "Batería baja", cuerpo: func(d Datos) string { return "SOC en " + n(d, "soc") + "% (reserva " + n(d, "reserva") + "%)" }},
		"bateria_llena": {titulo: "Batería llena", cuerpo: func(d Datos) string { return "La batería está al " + n(d, "soc") + "%" }},
		"resumen_diario": {titulo: "Resumen del día", cuerpo: func(d Datos) string {
			return "Producción " + n(d, "produccion") + " kWh · Consumo " + n(d, "consumo") + " kWh · Autoconsumo " + n(d, "autoconsumo") + "%"
		}},
		"resumen": {titulo: "Actividad en Helios", cuerpo: func(d Datos) string {
			return n(d, "total") + " alertas durante las horas de silencio"
		}},
	},
	LangEN: {
		"inversor_offline": {titulo: "Inverter offline", cuerpo: func(d Datos) string { return "The inverter is not responding" }},
		"inversor_ok":      {titulo: "Inverter recovered", cuerpo: func(d Datos) string { return "The inverter is back online" }},
		"fox_offline": {titulo: "Inverter offline", cuerpo: func(d Datos) string {
			return "The " + s(d, "nombre", "secondary") + " inverter is not reporting (daytime)"
		}},
		"fox_ok":        {titulo: "Inverter recovered", cuerpo: func(d Datos) string { return "The " + s(d, "nombre", "secondary") + " inverter is reporting again" }},
		"corte_red":     {titulo: "Grid outage", cuerpo: func(d Datos) string { return "Possible grid outage: running on battery/PV" }},
		"corte_red_ok":  {titulo: "Grid restored", cuerpo: func(d Datos) string { return "Grid power is back" }},
		"bateria_baja":  {titulo: "Low battery", cuerpo: func(d Datos) string { return "SOC at " + n(d, "soc") + "% (reserve " + n(d, "reserva") + "%)" }},
		"bateria_llena": {titulo: "Battery full", cuerpo: func(d Datos) string { return "The battery is at " + n(d, "soc") + "%" }},
		"resumen_diario": {titulo: "Daily summary", cuerpo: func(d Datos) string {
			return "Production " + n(d, "produccion") + " kWh · Consumption " + n(d, "consumo") + " kWh · Self-consumption " + n(d, "autoconsumo") + "%"
		}},
		"resumen": {titulo: "Helios activity", cuerpo: func(d Datos) string {
			return n(d, "total") + " alerts during your quiet hours"
		}},
	},
	LangZH: {
		"inversor_offline": {titulo: "逆变器离线", cuerpo: func(d Datos) string { return "逆变器无响应" }},
		"inversor_ok":      {titulo: "逆变器已恢复", cuerpo: func(d Datos) string { return "逆变器已重新上线" }},
		"fox_offline":      {titulo: "逆变器离线", cuerpo: func(d Datos) string { return s(d, "nombre", "辅助") + " 逆变器白天无数据" }},
		"fox_ok":           {titulo: "逆变器已恢复", cuerpo: func(d Datos) string { return s(d, "nombre", "辅助") + " 逆变器已恢复数据" }},
		"corte_red":        {titulo: "电网停电", cuerpo: func(d Datos) string { return "可能电网停电：正在使用电池/光伏供电" }},
		"corte_red_ok":     {titulo: "电网已恢复", cuerpo: func(d Datos) string { return "电网供电已恢复" }},
		"bateria_baja":     {titulo: "电池电量低", cuerpo: func(d Datos) string { return "SOC " + n(d, "soc") + "%（保留 " + n(d, "reserva") + "%）" }},
		"bateria_llena":    {titulo: "电池已满", cuerpo: func(d Datos) string { return "电池电量已达 " + n(d, "soc") + "%" }},
		"resumen_diario": {titulo: "每日总结", cuerpo: func(d Datos) string {
			return "发电 " + n(d, "produccion") + " kWh · 用电 " + n(d, "consumo") + " kWh · 自用 " + n(d, "autoconsumo") + "%"
		}},
		"resumen": {titulo: "Helios 动态", cuerpo: func(d Datos) string {
			return "免打扰期间有 " + n(d, "total") + " 条提醒"
		}},
	},
}

func (s *Service) langFor(userID string) Lang {
	lang, err := s.DB.UserLanguage(userID)
	if err != nil {
		return LangES
	}
	switch lang {
	case "en":
		return LangEN
	case "zh-CN", "zh":
		return LangZH
	}
	return LangES
}

// EnQuietHours comprueba la ventana de silencio en la zona del usuario.
func (s *Service) EnQuietHours(userID string) bool {
	q, err := s.DB.QuietHoursFor(userID)
	if err != nil || q == nil || q.QuietStart == nil || q.QuietEnd == nil {
		return false
	}
	tz := q.Tz
	if tz == "" {
		tz = "Europe/Madrid"
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		loc = time.Local
	}
	hora := time.Now().In(loc).Hour()

	start, end := *q.QuietStart, *q.QuietEnd
	if start <= end {
		return hora >= start && hora < end
	}
	return hora >= start || hora < end // cruza medianoche
}

func composePayload(lang Lang, tipo string, d Datos, url string) []byte {
	cat, ok := catalogo[lang]
	if !ok {
		cat = catalogo[LangES]
	}
	e, ok := cat[tipo]
	if !ok {
		e = cat["resumen"]
	}
	title := e.titulo
	body := e.cuerpo(d)
	payload, _ := json.Marshal(map[string]any{
		"title":    title,
		"body":     body,
		"url":      url,
		"tag":      tipo,
		"web_push": 8030,
		"notification": map[string]any{
			"title":    title,
			"body":     body,
			"navigate": url,
		},
	})
	return payload
}

type NotifyResult struct {
	Enviados   int `json:"enviados"`
	Borrados   int `json:"borrados"`
	Fallidos   int `json:"fallidos"`
	Pospuestos int `json:"pospuestos"`
	Omitidos   int `json:"omitidos"`
}

// NotifyUsers envía push a los usuarios indicados respetando preferencias y
// horas de silencio.
func (s *Service) NotifyUsers(userIDs []string, tipo string, datos Datos, opts Options) NotifyResult {
	severity := opts.Severity
	if severity == "" {
		severity = "normal"
	}
	url := opts.URL
	if url == "" {
		url = "/"
	}
	ttl := opts.TTL
	if ttl == 0 {
		if severity == "critical" {
			ttl = 3600
		} else {
			ttl = 21600
		}
	}

	res := NotifyResult{}
	seen := map[string]struct{}{}

	for _, userID := range userIDs {
		if _, ok := seen[userID]; ok {
			continue
		}
		seen[userID] = struct{}{}

		pref, err := s.DB.Preference(userID, tipo)
		if err != nil {
			res.Fallidos++
			continue
		}
		if pref != nil {
			if !pref.Enabled || severidadIdx(severity) < severidadIdx(pref.MinSeverity) {
				res.Omitidos++
				continue
			}
		}

		if severity != "critical" && s.EnQuietHours(userID) {
			dj, _ := json.Marshal(datos)
			s.DB.EnqueueNotification(newUUID(), userID, tipo, severity, string(dj), time.Now().UnixMilli())
			res.Pospuestos++
			continue
		}

		if !s.configured {
			res.Omitidos++
			continue
		}

		subs, err := s.DB.SubscriptionsByUser(userID)
		if err != nil {
			res.Fallidos++
			continue
		}
		if len(subs) == 0 {
			res.Omitidos++
			continue
		}

		lang := s.langFor(userID)
		payload := composePayload(lang, tipo, datos, url)

		urgency := "low"
		switch severity {
		case "critical":
			urgency = "high"
		case "high":
			urgency = "normal"
		}

		outcomes := make([]string, len(subs))
		var wg sync.WaitGroup
		sem := make(chan struct{}, maxConcurrentSends)
		for i, sub := range subs {
			wg.Add(1)
			go func(i int, sub db.PushSub) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				outcomes[i] = s.enviarAUna(sub, payload, ttl, urgency, tipo)
			}(i, sub)
		}
		wg.Wait()
		for _, outcome := range outcomes {
			switch outcome {
			case "ok":
				res.Enviados++
			case "borrada":
				res.Borrados++
			default:
				res.Fallidos++
			}
		}
	}
	return res
}

// enviarAUna: 404/410 borra la suscripción; 429/5xx reintenta con backoff.
func (s *Service) enviarAUna(sub db.PushSub, payload []byte, ttl int, urgency, topic string) string {
	if err := ValidateEndpoint(sub.Endpoint); err != nil {
		log.Printf("[push] suscripción %s descartada: %v", sub.ID, err)
		return "fallido"
	}
	dest := &webpush.Subscription{
		Endpoint: sub.Endpoint,
		Keys: webpush.Keys{
			P256dh: sub.P256dh,
			Auth:   sub.Auth,
		},
	}

	for intento := 1; intento <= 3; intento++ {
		opts := &webpush.Options{
			Subscriber:      s.Subject,
			VAPIDPublicKey:  s.PublicKey,
			VAPIDPrivateKey: s.PrivateKey,
			TTL:             ttl,
			Urgency:         webpush.Urgency(urgency),
			Topic:           topic,
			HTTPClient:      SendClient(),
		}
		status, err := s.sendFn(dest, payload, opts)
		if err == nil && status >= 200 && status < 300 {
			return "ok"
		}
		if status == 404 || status == 410 {
			s.DB.DeletePushByEndpoint(sub.Endpoint)
			return "borrada"
		}
		if status == 429 || status >= 500 {
			if intento < 3 {
				wait := time.Duration(500*(1<<intento)+rand.Intn(250)) * time.Millisecond
				time.Sleep(wait)
				continue
			}
			return "fallido"
		}
		if err != nil && status == 0 {
			if intento < 3 {
				time.Sleep(300 * time.Millisecond)
				continue
			}
		}
		log.Printf("[push] error status=%d sub=%s: %s", status, sub.ID, logSafeValue(redactEndpoint(sub.Endpoint)+" "+err.Error()))
		return "fallido"
	}
	return "fallido"
}

// NotifyAll avisa a todos los usuarios (alertas de sistema).
func (s *Service) NotifyAll(tipo string, datos Datos, opts Options) {
	ids, err := s.DB.AllUserIDs()
	if err != nil || len(ids) == 0 {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[push] panic en NotifyAll: %v", r)
		}
	}()
	s.NotifyUsers(ids, tipo, datos, opts)
}

// FlushNotificationQueue consolida la cola de horas de silencio.
func (s *Service) FlushNotificationQueue() {
	groups, err := s.DB.QueueGroups()
	if err != nil {
		return
	}
	for _, g := range groups {
		if s.EnQuietHours(g.UserID) {
			continue
		}
		datos := Datos{"total": float64(g.Total)}
		s.NotifyUsers([]string{g.UserID}, "resumen", datos, Options{Severity: g.Severity})
		s.DB.DeleteQueueFor(g.UserID, g.Tipo)
	}
}

func newUUID() string {
	b := make([]byte, 16)
	if _, err := randRead(b); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	const hex = "0123456789abcdef"
	var out strings.Builder
	for i, v := range b {
		if i == 4 || i == 6 || i == 8 || i == 10 {
			out.WriteByte('-')
		}
		out.WriteByte(hex[v>>4])
		out.WriteByte(hex[v&0x0f])
	}
	return out.String()
}

var errNoSubs = errors.New("sin suscripciones")
var _ = context.Background
var _ = errNoSubs

func randRead(b []byte) (int, error) {
	return cryptoRandRead(b)
}

// cryptoRandRead aísla crypto/rand para poder sustituirlo en tests.
func cryptoRandRead(b []byte) (int, error) { return cryptorand.Read(b) }
