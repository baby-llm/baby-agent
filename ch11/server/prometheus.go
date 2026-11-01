package server

import "github.com/prometheus/client_golang/prometheus/promhttp"
import "net/http"

func prometheusHandler() http.Handler { return promhttp.Handler() }
