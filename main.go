package main

import (
	"net/http"
	"os"
	"strconv"

	"github.com/go-chi/chi/v5"
	log "github.com/sirupsen/logrus"
	"github.com/waldner/external-dns-webhook-he/pkg/client"
	"github.com/waldner/external-dns-webhook-he/pkg/config"
	"github.com/waldner/external-dns-webhook-he/pkg/provider"
	"github.com/waldner/external-dns-webhook-he/pkg/webhook"
)

var version = "0.0.2"

func initLog() {

	log.SetFormatter(&log.JSONFormatter{
		DisableHTMLEscape: true,
	})

	// this can be a numeric value, or a string like "debug", "trace", etc
	level := os.Getenv("WEBHOOK_HE_LOG_LEVEL")
	if level == "" {
		log.SetLevel(log.InfoLevel)
	} else {
		if levelInt, err := strconv.Atoi(level); err == nil {
			log.SetLevel(log.Level(uint32(levelInt)))
		} else {

			levelInt, err := log.ParseLevel(level)
			if err != nil {
				log.SetLevel(log.InfoLevel)
				log.Warnf("Invalid log level '%s', defaulting to info", level)
			} else {
				log.SetLevel(levelInt)
			}
		}
	}
}

func main() {
	initLog()
	log.WithFields(log.Fields{"version": version}).Info("Starting external-dns-webhook-he")

	heConfig, domainFilter, err := config.NewConfig()
	if err != nil {
		log.Fatal(err)
	}

	client, err := client.NewClient(heConfig)
	if err != nil {
		log.Fatal(err)
	}

	provider, err := provider.NewProvider(client, domainFilter)
	if err != nil {
		log.Fatal(err)
	}

	hook, err := webhook.NewWebhook(provider)
	if err != nil {
		log.Fatal(err)
	}

	r := chi.NewRouter()

	// healthcheck as middleware
	r.Use(webhook.Health)

	r.Get("/", hook.Negotiate)
	r.Get("/records", hook.Records)
	r.Post("/adjustendpoints", hook.AdjustEndpoints)
	r.Post("/records", hook.ApplyChanges)

	http.ListenAndServe(":3333", r)
}
