package main

import (
	"fmt"
	"net/http"
	"sync"
	"time"
)

type Metrics struct {
	// TotalRequests is the total number of requests by status code
	TotalRequests map[int]uint64
	// TotalBytes is the total number of bytes sent by status code
	TotalBytes map[int]uint64

	runId string
	lock  sync.RWMutex
}

// NewMetrics creates a new Metrics object
func NewMetrics() *Metrics {
	return &Metrics{
		TotalRequests: map[int]uint64{http.StatusOK: 0},
		TotalBytes:    map[int]uint64{http.StatusOK: 0},
		runId:         fmt.Sprintf("%d", time.Now().UnixNano()),
		lock:          sync.RWMutex{},
	}
}

type loggedResponseWriter struct {
	http.ResponseWriter
	StatusCode       int
	BytesTransferred uint64
}

func NewLoggedResponseWriter(w http.ResponseWriter) *loggedResponseWriter {
	return &loggedResponseWriter{w, http.StatusOK, 0}
}

func (w *loggedResponseWriter) WriteHeader(code int) {
	w.StatusCode = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *loggedResponseWriter) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	w.BytesTransferred += uint64(n)
	return n, err
}

// WithMetrics wraps a handler with metrics
func (m *Metrics) WithMetrics(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Create a logged response writer
		lw := NewLoggedResponseWriter(w)

		// Call the handler
		handler.ServeHTTP(lw, r)

		// Update the metrics
		m.lock.Lock()
		defer m.lock.Unlock()

		m.TotalRequests[lw.StatusCode]++
		m.TotalBytes[lw.StatusCode] += lw.BytesTransferred
	}
}

// ServeHTTP serves the metrics
func (m *Metrics) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.lock.RLock()
	defer m.lock.RUnlock()

	appName := "content_server_go"
	commonTags := fmt.Sprintf("app=\"%s\",run_id=\"%s\"", appName, m.runId)

	w.Write([]byte(
		fmt.Sprintf("%s_instance{%s} 1\n", appName, commonTags),
	))

	for statusCode, count := range m.TotalRequests {
		w.Write([]byte(
			fmt.Sprintf("%s_total_requests{%s,status_code=\"%d\"} %d\n", appName, commonTags, statusCode, count),
		))
	}

	for statusCode, bytes := range m.TotalBytes {
		w.Write([]byte(
			fmt.Sprintf("%s_total_bytes{%s,status_code=\"%d\"} %d\n", appName, commonTags, statusCode, bytes),
		))
	}
}
