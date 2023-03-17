package util

import (
	"log"
	"net/http"
	"net/http/pprof"
	"os"
	"strings"
)

const (
	// ENABLE_PROFILING is set to True to start profilers.
	enableProfilingEnv string = "ENABLE_PROFILING"
)

// IsProfilingEnabled checks if profiling is enabled.
func IsProfilingEnabled() bool {
	val, found := os.LookupEnv(enableProfilingEnv)
	if !found {
		return false
	}

	return strings.ToLower(val) == "true"
}

// StartProfilers starts a pprof profiling server at the given address.
func StartProfilers(addr string) {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	// #nosec G114
	log.Fatal(http.ListenAndServe(addr, mux))
}
