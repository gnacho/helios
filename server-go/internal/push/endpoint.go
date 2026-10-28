package push

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
)

const (
	sendTimeout        = 15 * time.Second
	maxConcurrentSends = 4
)

var errUnsafeEndpoint = errors.New("endpoint web push no permitido")

// ValidateEndpoint only accepts absolute HTTPS URLs whose host is not a
// private, loopback, link-local, CGNAT or otherwise special address.
func ValidateEndpoint(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("%w: %v", errUnsafeEndpoint, err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("%w: esquema %q", errUnsafeEndpoint, u.Scheme)
	}
	if u.User != nil {
		return fmt.Errorf("%w: userinfo", errUnsafeEndpoint)
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("%w: sin host", errUnsafeEndpoint)
	}
	if strings.HasSuffix(strings.ToLower(host), ".local") {
		return fmt.Errorf("%w: dominio local", errUnsafeEndpoint)
	}
	if ip := net.ParseIP(host); ip != nil {
		if !isPublicIP(ip) {
			return fmt.Errorf("%w: ip no pública", errUnsafeEndpoint)
		}
	}
	return nil
}

func isPublicIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() || ip.IsMulticast() {
		return false
	}
	if v4 := ip.To4(); v4 != nil {
		switch {
		case v4[0] == 100 && v4[1]&0xc0 == 64: // 100.64.0.0/10 CGNAT
			return false
		case v4[0] == 192 && v4[1] == 0 && v4[2] == 0: // 192.0.0.0/24
			return false
		case v4[0] == 198 && (v4[1] == 18 || v4[1] == 19): // benchmarking
			return false
		case v4[0] == 192 && v4[1] == 0 && v4[2] == 2: // TEST-NET-1
			return false
		case v4[0] == 198 && v4[1] == 51 && v4[2] == 100: // TEST-NET-2
			return false
		case v4[0] == 203 && v4[1] == 0 && v4[2] == 113: // TEST-NET-3
			return false
		case v4[0] >= 240: // reservado
			return false
		}
	}
	return true
}

// SendClient returns an HTTP client that enforces the total timeout and
// refuses redirects, so a 30x cannot bounce the request to an internal host.
func SendClient() *http.Client {
	dialer := &net.Dialer{
		Timeout:   5 * time.Second,
		KeepAlive: 30 * time.Second,
		Control: func(network, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return errUnsafeEndpoint
			}
			ip := net.ParseIP(host)
			if ip == nil || !isPublicIP(ip) {
				return fmt.Errorf("%w: conexión a %s", errUnsafeEndpoint, host)
			}
			return nil
		},
	}
	return &http.Client{
		Timeout: sendTimeout,
		Transport: &http.Transport{
			DialContext:           dialer.DialContext,
			TLSHandshakeTimeout:   5 * time.Second,
			ResponseHeaderTimeout: 10 * time.Second,
			MaxIdleConns:          maxConcurrentSends,
			MaxIdleConnsPerHost:   2,
			IdleConnTimeout:       30 * time.Second,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func defaultSendFn() SendFn {
	return func(sub *webpush.Subscription, payload []byte, opts *webpush.Options) (int, error) {
		resp, err := webpush.SendNotification(payload, sub, opts)
		if err != nil {
			return 0, err
		}
		defer resp.Body.Close()
		return resp.StatusCode, nil
	}
}
