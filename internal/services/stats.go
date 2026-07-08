package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// Traffic + IP-info event names the frontend subscribes to.
const (
	TrafficStatsEvent = "connection:stats"
	IPInfoEvent       = "connection:ipinfo"
)

// TrafficStats is the live tunnel traffic snapshot. Rx/Tx are totals; the rates are
// the per-second deltas. Downlink = rx, Uplink = tx.
type TrafficStats struct {
	RxBytes int64 `json:"rxBytes"`
	TxBytes int64 `json:"txBytes"`
	RxRate  int64 `json:"rxRate"`
	TxRate  int64 `json:"txRate"`
}

// IPInfo is the public exit-IP lookup (through the tunnel once connected).
// CountryCode is the ISO 3166-1 alpha-2 code the frontend turns into a flag emoji.
type IPInfo struct {
	IP          string `json:"ip"`
	Country     string `json:"country"`
	CountryCode string `json:"countryCode"`
	Provider    string `json:"provider"`
}

// startStatsPoller samples the WireGuard interface counters once a second and emits
// the totals plus per-second rates until ctx is cancelled. Counters come from sysfs
// (world-readable), so no privilege is needed here.
func (s *ConnectionService) startStatsPoller(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		var lastRx, lastTx int64
		first := true
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				rx, tx, ok := wgInterfaceStats(wgInterface)
				if !ok {
					continue
				}
				stats := TrafficStats{RxBytes: rx, TxBytes: tx}
				if !first {
					stats.RxRate = nonNegative(rx - lastRx)
					stats.TxRate = nonNegative(tx - lastTx)
				}
				lastRx, lastTx, first = rx, tx, false
				s.emit2(TrafficStatsEvent, stats)
			}
		}
	}()
}

// IPInfo returns the last looked-up exit IP info.
func (s *ConnectionService) IPInfo() IPInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ipInfo
}

// RefreshIPInfo fetches the public exit IP / country / provider (through whatever
// path is currently active) and caches + emits it.
func (s *ConnectionService) RefreshIPInfo() (IPInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	info, err := fetchIPInfo(ctx)
	if err != nil {
		log.Printf("[ipinfo] refresh failed: %v", err)
		return IPInfo{}, err
	}
	log.Printf("[ipinfo] refresh -> ip=%s cc=%s provider=%q", info.IP, info.CountryCode, info.Provider)
	s.mu.Lock()
	s.ipInfo = info
	s.mu.Unlock()
	s.emit2(IPInfoEvent, info)
	return info, nil
}

func (s *ConnectionService) refreshIPInfoAsync(ctx context.Context) {
	go func() {
		// The exit IP only changes once the WG handshake completes and the tunnel
		// actually carries traffic, which can lag WGUp by several seconds. A single
		// early lookup reads the physical IP and sticks; re-fetch a few times with
		// growing delays so the UI settles on the tunnel exit IP.
		for _, delay := range []time.Duration{2 * time.Second, 6 * time.Second, 12 * time.Second} {
			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
			}
			if ctx.Err() != nil {
				return
			}
			_, _ = s.RefreshIPInfo()
		}
	}()
}

// wgInterfaceStats reads the interface rx/tx byte counters from sysfs. For a
// WireGuard interface these count the inner (tunneled) traffic: rx = download, tx =
// upload. Returns ok=false when the interface is absent (not connected).
func wgInterfaceStats(iface string) (rx, tx int64, ok bool) {
	rx, err1 := readCounter("/sys/class/net/" + iface + "/statistics/rx_bytes")
	tx, err2 := readCounter("/sys/class/net/" + iface + "/statistics/tx_bytes")
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return rx, tx, true
}

func readCounter(path string) (int64, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
}

func nonNegative(v int64) int64 {
	if v < 0 {
		return 0
	}
	return v
}

// fetchIPInfo tries public IP services, in order, returning
// the first that yields an IP.
func fetchIPInfo(ctx context.Context) (IPInfo, error) {
	type parser func([]byte) IPInfo
	endpoints := []struct {
		url   string
		parse parser
	}{
		{"https://ipwho.is/", parseIPWho},
		{"https://ipapi.co/json/", parseIPAPI},
		{"https://ipinfo.io/json", parseIPInfoIO},
	}
	var lastErr error
	// A dedicated non-pooling transport: with the shared http.DefaultTransport, an idle
	// connection opened before the app process joined the whitelist tunnel cgroup gets
	// reused, so the lookup keeps exiting on the physical link. DisableKeepAlives forces
	// a fresh socket per request, which is created in (and marked by) the current cgroup.
	client := &http.Client{
		Timeout:   6 * time.Second,
		Transport: &http.Transport{DisableKeepAlives: true, Proxy: http.ProxyFromEnvironment},
	}
	for _, e := range endpoints {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.url, nil)
		if err != nil {
			lastErr = err
			continue
		}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}
		if info := e.parse(body); strings.TrimSpace(info.IP) != "" {
			return info, nil
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("ip lookup failed")
	}
	return IPInfo{}, lastErr
}

func parseIPWho(body []byte) IPInfo {
	var v struct {
		IP          string `json:"ip"`
		Country     string `json:"country"`
		CountryCode string `json:"country_code"`
		Connection  struct {
			ISP string `json:"isp"`
			Org string `json:"org"`
		} `json:"connection"`
	}
	if json.Unmarshal(body, &v) != nil {
		return IPInfo{}
	}
	return IPInfo{IP: v.IP, Country: v.Country, CountryCode: v.CountryCode, Provider: firstNonBlank(v.Connection.ISP, v.Connection.Org)}
}

func parseIPAPI(body []byte) IPInfo {
	var v struct {
		IP          string `json:"ip"`
		Country     string `json:"country_name"`
		CountryCode string `json:"country"`
		Org         string `json:"org"`
	}
	if json.Unmarshal(body, &v) != nil {
		return IPInfo{}
	}
	return IPInfo{IP: v.IP, Country: v.Country, CountryCode: v.CountryCode, Provider: v.Org}
}

func parseIPInfoIO(body []byte) IPInfo {
	var v struct {
		IP      string `json:"ip"`
		Country string `json:"country"` // ipinfo.io returns the 2-letter code here
		Org     string `json:"org"`
	}
	if json.Unmarshal(body, &v) != nil {
		return IPInfo{}
	}
	return IPInfo{IP: v.IP, CountryCode: v.Country, Provider: v.Org}
}

func firstNonBlank(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
