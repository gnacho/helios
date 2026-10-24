// Package version contiene la versión del backend. Se inyecta en el build con
// -ldflags "-X helios-server/internal/version.Version=x.y.z" (ver CI).
package version

// Version es la versión publicada del backend.
var Version = "1.1.7"
