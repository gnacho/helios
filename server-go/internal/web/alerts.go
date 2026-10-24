package web

// Alert dismissal: each live alert can be cleared from the bell with its
// check. Cleared IDs are stored per user in the KV (a JSON map id -> unix
// time) and filtered out of the bell; when an alert stops being live its ID is
// pruned, so the same condition showing up again later is shown again.

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/starfederation/datastar-go/datastar"

	"helios-server/internal/auth"
	"helios-server/internal/solar"
)

const alertsDismissedPrefix = "alertsDismissed:"

// alertBellSlots are the two bells the shell renders (desktop and mobile), so
// the dismissal patches both.
var alertBellSlots = []string{"desk", "mob"}

// alertsSeverityClass is the dot color of one alert.
func alertsSeverityClass(sev string) string {
	switch sev {
	case "critical":
		return "bg-rose-500"
	case "warning":
		return "bg-amber-500"
	default:
		return "bg-sky-500"
	}
}

// alertsDotClass is the bell dot color for the worst live severity.
func alertsDotClass(level string) string {
	switch level {
	case "critical":
		return "bg-rose-500"
	case "warning":
		return "bg-amber-500"
	case "info":
		return "bg-sky-500"
	}
	return ""
}

// alertsLevel returns the worst severity of the list ("" when empty).
func alertsLevel(alerts []solar.LiveAlert) string {
	level := ""
	for _, a := range alerts {
		switch a.Severity {
		case "critical":
			return "critical"
		case "warning":
			if level != "critical" {
				level = "warning"
			}
		default:
			if level == "" {
				level = "info"
			}
		}
	}
	return level
}

// dismissedAlerts is the set of alert IDs the user cleared (id -> unix).
func (u *UI) dismissedAlerts(userID string) map[string]int64 {
	out := map[string]int64{}
	if u.DB == nil || userID == "" {
		return out
	}
	raw, _ := u.DB.KVGet(alertsDismissedPrefix + userID)
	if raw == "" {
		return out
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return map[string]int64{}
	}
	return out
}

// liveAlerts is the bell snapshot without the alerts this user cleared. IDs of
// alerts that are no longer live are pruned so they show up again next time HA
// raises them.
func (u *UI) liveAlerts(userID string) ([]solar.LiveAlert, string) {
	all, _ := u.alertsSnapshot()
	gone := u.dismissedAlerts(userID)
	if len(gone) == 0 {
		return all, alertsLevel(all)
	}
	alive := make(map[string]bool, len(all))
	kept := make([]solar.LiveAlert, 0, len(all))
	for _, a := range all {
		alive[a.ID] = true
		if _, cleared := gone[a.ID]; !cleared {
			kept = append(kept, a)
		}
	}
	pruned := false
	for id := range gone {
		if !alive[id] {
			delete(gone, id)
			pruned = true
		}
	}
	if pruned && u.DB != nil {
		if b, err := json.Marshal(gone); err == nil {
			u.DB.KVSet(alertsDismissedPrefix+userID, string(b))
		}
	}
	return kept, alertsLevel(kept)
}

// dismissAlertClick posts the dismissal of one alert without reloading the page.
func dismissAlertClick(id string) string {
	return "@post('/ui/alerts/dismiss?id=" + url.QueryEscape(id) + "')"
}

// handleAlertDismiss clears one alert and patches the bell in place: the dot
// and the list, so the open panel keeps its state.
func (u *UI) handleAlertDismiss(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFrom(r.Context())
	// The id travels in the query: Datastar posts its signals as JSON, so a
	// form field would never reach the handler and the body must stay intact
	// for datastar.NewSSE.
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if !ok || userID == "" || id == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	gone := u.dismissedAlerts(userID)
	gone[id] = time.Now().Unix()
	if u.DB != nil {
		if b, err := json.Marshal(gone); err == nil {
			u.DB.KVSet(alertsDismissedPrefix+userID, string(b))
		}
	}
	alerts, level := u.liveAlerts(userID)
	lang := u.userLang(userID)
	gen := datastar.NewSSE(w, r)
	// The shell renders two bells (desktop header and mobile bar): patch both.
	for _, slot := range alertBellSlots {
		_ = gen.PatchElementTempl(alertsDot(level, slot), datastar.WithSelectorID("alerts-dot-"+slot), datastar.WithModeOuter())
		_ = gen.PatchElementTempl(alertsList(lang, alerts, slot), datastar.WithSelectorID("alerts-list-"+slot), datastar.WithModeOuter())
	}
}
