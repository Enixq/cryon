package core

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"Cryon2/internal/update"
)

func (a *App) handleAPIDeviceState(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		a.remoteStateMu.RLock()
		state := append([]byte(nil), a.remoteState...)
		a.remoteStateMu.RUnlock()
		if len(state) == 0 {
			writeAPIJSON(w, http.StatusOK, map[string]any{})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(state)
		return
	}
	if r.Method != http.MethodPut {
		writeAPIError(w, http.StatusMethodNotAllowed, "требуется GET или PUT")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 2<<20))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "некорректное состояние устройства")
		return
	}
	var probe map[string]any
	if json.Unmarshal(body, &probe) != nil {
		writeAPIError(w, http.StatusBadRequest, "некорректный JSON")
		return
	}
	a.remoteStateMu.Lock()
	a.remoteRevision++
	probe["revision"] = a.remoteRevision
	encoded, err := json.Marshal(probe)
	if err != nil {
		a.remoteStateMu.Unlock()
		writeAPIError(w, http.StatusBadRequest, "некорректное состояние устройства")
		return
	}
	a.remoteState = append(a.remoteState[:0], encoded...)
	a.remoteStateMu.Unlock()
	w.WriteHeader(204)
}

const (
	deviceDiscoveryPort = 8898
	discoveryMagic      = "CRYON_DISCOVER_V1"
)

type discoveredDevice struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	URL     string `json:"url"`
	Version string `json:"version"`
}

// startDeviceDiscoveryResponder lets a desktop and an Android instance find
// one another without typing an IP address. It is deliberately a tiny UDP
// responder; all actual data and control still use the authenticated-by-LAN
// HTTP API on port 8899.
func startDeviceDiscoveryResponder() {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{Port: deviceDiscoveryPort})
	if err != nil {
		return
	}
	go func() {
		defer conn.Close()
		buf := make([]byte, 256)
		for {
			_ = conn.SetReadDeadline(time.Now().Add(2 * time.Minute))
			n, addr, readErr := conn.ReadFromUDP(buf)
			if readErr != nil {
				if ne, ok := readErr.(net.Error); ok && ne.Timeout() {
					continue
				}
				return
			}
			if string(buf[:n]) != discoveryMagic {
				continue
			}
			ip := localIPv4For(addr.IP)
			if ip == "" {
				continue
			}
			payload, _ := json.Marshal(discoveredDevice{
				ID:      "cryon-" + ip,
				Name:    "Cryon",
				URL:     "http://" + ip + ":" + fmt.Sprint(lanServerPort),
				Version: update.Version,
			})
			_, _ = conn.WriteToUDP(payload, addr)
		}
	}()
}

func localIPv4For(peer net.IP) string {
	interfaces, _ := net.Interfaces()
	for _, iface := range interfaces {
		addrs, _ := iface.Addrs()
		for _, raw := range addrs {
			ip, _, err := net.ParseCIDR(raw.String())
			if err != nil || ip.To4() == nil || ip.IsLoopback() || !sameIPv4Subnet(ip, peer) {
				continue
			}
			return ip.To4().String()
		}
	}
	return ""
}

func sameIPv4Subnet(a, b net.IP) bool {
	a4, b4 := a.To4(), b.To4()
	return a4 != nil && b4 != nil && a4[0] == b4[0] && a4[1] == b4[1] && a4[2] == b4[2]
}

func discoverDevices(ctx context.Context, wifiIPv4 string) []discoveredDevice {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return nil
	}
	defer conn.Close()
	_ = conn.SetWriteBuffer(4096)
	_ = conn.SetReadBuffer(4096)
	_, _ = conn.WriteToUDP([]byte(discoveryMagic), &net.UDPAddr{IP: net.IPv4bcast, Port: deviceDiscoveryPort})
	localIPs := map[string]bool{}
	if ip := net.ParseIP(wifiIPv4).To4(); ip != nil && ip.IsPrivate() {
		localIPs[ip.String()] = true
	}
	if addrs, err := net.InterfaceAddrs(); err == nil {
		for _, addr := range addrs {
			ip, network, parseErr := net.ParseCIDR(addr.String())
			ip = ip.To4()
			if parseErr != nil || ip == nil || ip.IsLoopback() {
				continue
			}
			localIPs[ip.String()] = true
			mask := network.Mask
			if len(mask) != 4 {
				continue
			}
			broadcast := net.IPv4(ip[0]|^mask[0], ip[1]|^mask[1], ip[2]|^mask[2], ip[3]|^mask[3])
			_, _ = conn.WriteToUDP([]byte(discoveryMagic), &net.UDPAddr{IP: broadcast, Port: deviceDiscoveryPort})
		}
	}
	deadline := time.Now().Add(900 * time.Millisecond)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	seen := map[string]bool{}
	out := make([]discoveredDevice, 0)
	// Broadcast can be blocked by router isolation or a desktop firewall even
	// when the Cryon TCP port itself is reachable. Probe the local IPv4 subnet
	// concurrently as a bounded fallback; never scan public addresses.
	probed := make(chan discoveredDevice, 256)
	var probes sync.WaitGroup
	sem := make(chan struct{}, 64)
	probeClient := &http.Client{Transport: &http.Transport{Proxy: nil, DisableKeepAlives: true}}
	for address := range localIPs {
		ip := net.ParseIP(address).To4()
		if ip == nil || !ip.IsPrivate() {
			continue
		}
		for n := 1; n < 255; n++ {
			target := net.IPv4(ip[0], ip[1], ip[2], byte(n)).String()
			if localIPs[target] {
				continue
			}
			probes.Add(1)
			go func() {
				defer probes.Done()
				select {
				case sem <- struct{}{}:
				case <-ctx.Done():
					return
				}
				defer func() { <-sem }()
				probeCtx, cancel := context.WithTimeout(ctx, 550*time.Millisecond)
				defer cancel()
				url := fmt.Sprintf("http://%s:%d", target, lanServerPort)
				req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, url+"/v1/system/health", nil)
				if err != nil {
					return
				}
				resp, err := probeClient.Do(req)
				if err != nil {
					return
				}
				defer resp.Body.Close()
				var health struct {
					Status string `json:"status"`
					API    string `json:"api"`
				}
				if resp.StatusCode == http.StatusOK && json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&health) == nil && health.Status == "ok" && health.API == "v1" {
					probed <- discoveredDevice{ID: "cryon-" + target, Name: "Cryon", URL: url}
				}
			}()
		}
	}
	for time.Now().Before(deadline) {
		_ = conn.SetReadDeadline(deadline)
		var payload []byte = make([]byte, 1024)
		n, peer, readErr := conn.ReadFromUDP(payload)
		if readErr != nil {
			break
		}
		var device discoveredDevice
		if peer.IP.IsLoopback() || localIPs[peer.IP.String()] {
			continue
		}
		if json.Unmarshal(payload[:n], &device) == nil && device.URL != "" && !seen[device.URL] {
			seen[device.URL] = true
			out = append(out, device)
		}
	}
	probes.Wait()
	close(probed)
	for device := range probed {
		if !seen[device.URL] {
			seen[device.URL] = true
			out = append(out, device)
		}
	}
	return out
}
