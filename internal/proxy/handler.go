package proxy

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ProxyHandler handles proxy requests.
type ProxyHandler struct{}

func (h *ProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host := r.Host

	// Strip port if present
	if idx := strings.Index(host, ":"); idx > 0 {
		host = host[:idx]
	}

	// Parse hostname: <canvas>.<branch>.dlio.localhost
	parts := strings.Split(host, ".")

	// Must have at least 4 parts and end with dlio.localhost
	if len(parts) < 4 || parts[len(parts)-2] != "dlio" || parts[len(parts)-1] != "localhost" {
		http.Error(w, fmt.Sprintf("Invalid hostname format: %s\nExpected: <canvas>.<branch>.dlio.localhost", host), http.StatusBadRequest)
		return
	}

	// Find dlio index and extract branch name
	dlioIdx := -1
	for i, p := range parts {
		if p == "dlio" {
			dlioIdx = i
			break
		}
	}

	if dlioIdx < 2 {
		http.Error(w, fmt.Sprintf("Invalid hostname format: %s", host), http.StatusBadRequest)
		return
	}

	branchName := parts[dlioIdx-1]
	canvasParts := append(parts[:dlioIdx-1], parts[dlioIdx:]...)
	canvasHost := strings.Join(canvasParts, ".")

	// Look up port for branch
	port, ok := BranchPorts[branchName]
	if !ok {
		// Refresh cache and try again
		RefreshBranchPorts()
		port, ok = BranchPorts[branchName]
	}

	if !ok {
		http.Error(w, fmt.Sprintf("Branch '%s' not running.\nRunning branches: %v", branchName, getBranchNames()), http.StatusNotFound)
		return
	}

	// Forward request
	targetURL := fmt.Sprintf("http://localhost:%d%s", port, r.URL.Path)
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	proxyReq, err := http.NewRequest(r.Method, targetURL, r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("Proxy error: %v", err), http.StatusInternalServerError)
		return
	}

	// Copy headers, replacing Host
	for key, values := range r.Header {
		if strings.ToLower(key) == "host" {
			continue
		}
		for _, value := range values {
			proxyReq.Header.Add(key, value)
		}
	}
	proxyReq.Header.Set("Host", canvasHost)

	// Make request
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(proxyReq)
	if err != nil {
		http.Error(w, fmt.Sprintf("Backend error: %v", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Copy response headers
	for key, values := range resp.Header {
		if strings.ToLower(key) == "transfer-encoding" {
			continue
		}
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	// Copy response
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func getBranchNames() []string {
	var names []string
	for name := range BranchPorts {
		names = append(names, name)
	}
	return names
}
