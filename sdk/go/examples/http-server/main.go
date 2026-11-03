package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	report "github.com/acugis/acugis-report-go"
)

func main() {
	client, err := report.NewClientFromEnv()
	if err != nil {
		log.Fatalf("client: %v", err)
	}

	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	http.HandleFunc("/me", func(w http.ResponseWriter, r *http.Request) {
		me, err := client.Auth.Me(r.Context())
		if err != nil {
			http.Error(w, err.Error(), statusOf(err))
			return
		}
		writeJSON(w, me)
	})

	http.HandleFunc("/reports", func(w http.ResponseWriter, r *http.Request) {
		list, err := client.Reports.List(r.Context())
		if err != nil {
			http.Error(w, err.Error(), statusOf(err))
			return
		}
		writeJSON(w, list)
	})

	http.HandleFunc("/run/", func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Path[len("/run/"):]
		if key == "" {
			http.Error(w, "missing report key", http.StatusBadRequest)
			return
		}
		format := r.URL.Query().Get("format")
		if format == "" {
			format = "pdf"
		}
		res, err := client.Reports.Run(context.Background(), key, &report.RunOptions{Format: format})
		if err != nil {
			http.Error(w, err.Error(), statusOf(err))
			return
		}
		if res.ContentType != "" {
			w.Header().Set("Content-Type", res.ContentType)
		}
		w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", key+"."+format))
		_, _ = w.Write(res.Data)
	})

	addr := ":8088"
	if v := os.Getenv("PORT"); v != "" {
		addr = ":" + v
	}
	log.Printf("listening on %s (ACUGIS_REPORT_URL=%s)", addr, os.Getenv(report.EnvURL))
	log.Fatal(http.ListenAndServe(addr, nil))
}

func statusOf(err error) int {
	type statusCoder interface{ StatusCode() int }
	if sc, ok := err.(statusCoder); ok && sc.StatusCode() > 0 {
		return sc.StatusCode()
	}
	return http.StatusBadGateway
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
