package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/coreos/go-systemd/activation"
	"github.com/go-openapi/runtime"
	"github.com/gotify/go-api-client/v2/auth"
	"github.com/gotify/go-api-client/v2/client"
	"github.com/gotify/go-api-client/v2/client/application"
	"github.com/gotify/go-api-client/v2/client/message"
	"github.com/gotify/go-api-client/v2/gotify"
	"github.com/gotify/go-api-client/v2/models"
	"github.com/prometheus/alertmanager/template"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

var (
	gotifyUrl    = flag.String("url", "", "Gotify server URL")
	gotifyToken  = flag.String("token", "", "Gotify app secret token")
	gotifyCToken = flag.String("ctoken", "", "Gotify client secret token for --resolved=delete")
	resolved     = flag.String("resolved", "notify", "Behavior for resolved alerts; either 'notify' (default), 'ignore' or 'delete")
	exitAfter    = flag.Int("exitafter", 0, "How long to wait before quiting after handling a request; 0 to stay up forever")

	gotifyClient *client.GotifyREST
	appId        int64 = 0
	pleaseQuit         = make(chan struct{})
)

const (
	extraKey = "am2gotify/fp"
)

func Handle(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	if *exitAfter != 0 {
		defer func() {
			pleaseQuit <- struct{}{}
		}()
	}

	dec := json.NewDecoder(r.Body)
	var m template.Data
	if err := dec.Decode(&m); err != nil {
		http.Error(w, fmt.Sprintf("error decoding AM hook message: %v", err), 400)
		return
	}

	var lastErr error = nil
	for _, a := range m.Alerts {
		prefix := ""
		if a.Status == "resolved" {
			if *resolved == "ignore" {
				continue
			}
			if *resolved == "delete" {
				deleteMessage(w, a)
				continue
			}
			prefix = "resolved"
		} else /* firing */ {
			prefix = a.Labels["severity"]
			if prefix == "" {
				prefix = "warning"
			}
		}
		instance := a.Labels["instance"]
		if instance != "" {
			instance = instance + ": "
		}
		priority := 5
		if pp, err := strconv.Atoi(a.Labels["p"]); err == nil {
			priority = pp
		}
		if _, err := gotifyClient.Message.CreateMessage(&message.CreateMessageParams{
			Body: &models.MessageExternal{
				Title:    fmt.Sprintf("[%s] %s", a.Status, a.Annotations["summary"]),
				Message:  instance + a.Annotations["description"],
				Priority: priority,
				Extras:   map[string]interface{}{extraKey: a.Fingerprint},
			},
			Context: context.Background(),
		}, tokenAuth()); err != nil {
			lastErr = err
		}
	}
	if lastErr != nil {
		http.Error(w, fmt.Sprintf("error sending Gotify message: %v", lastErr), 500)
		return
	}
	w.WriteHeader(204)
}

func deleteMessage(w http.ResponseWriter, a template.Alert) {
	messages, err := gotifyClient.Message.GetAppMessages(&message.GetAppMessagesParams{
		ID:      appId,
		Context: context.Background(),
	}, cTokenAuth())
	if err != nil {
		http.Error(w, fmt.Sprintf("GetAppMessages failed: %v", err), 500)
		return
	}
	for _, m := range messages.Payload.Messages {
		if m.Extras[extraKey] == a.Fingerprint {
			_, _ = gotifyClient.Message.DeleteMessage(&message.DeleteMessageParams{
				ID:      int64(m.ID),
				Context: context.Background(),
			}, cTokenAuth())
		}
	}
}

func tokenAuth() runtime.ClientAuthInfoWriter {
	return auth.TokenAuth(*gotifyToken)
}

func cTokenAuth() runtime.ClientAuthInfoWriter {
	return auth.TokenAuth(*gotifyCToken)
}

func main() {
	flag.Parse()
	listeners, err := activation.Listeners()
	if err != nil {
		log.Panicf("cannot retrieve listeners: %s", err)
	}
	if len(listeners) != 1 {
		log.Panicf("unexpected number of socket activation (%d != 1)",
			len(listeners))
	}

	u, err := url.Parse(*gotifyUrl)
	if err != nil {
		log.Panicf("invalid --url: %s", *gotifyUrl)
	}
	gotifyClient = gotify.NewClient(u, &http.Client{Timeout: 10 * time.Second})
	versionResponse, err := gotifyClient.Version.GetVersion(nil)
	if err != nil {
		log.Fatal("could not request version ", err)
		return
	}
	version := versionResponse.Payload
	log.Printf("Gotify version: %v", *version)

	if *resolved == "delete" {
		apps, err := gotifyClient.Application.GetApps(&application.GetAppsParams{
			Context: context.Background(),
		}, cTokenAuth())
		if err != nil {
			log.Fatalf("for --resolved=delete: unable to retrieve app list: %v, did you provide a valid --ctoken?", err)
		}
		for _, app := range apps.Payload {
			if app.Token == *gotifyToken {
				appId = int64(app.ID)
				break
			}
		}
		if appId == 0 {
			log.Fatalf("for --resolved=delete: could not find application matching provided --token")
		} else {
			log.Printf("for --resolved=delete: app ID is %v", appId)
		}
	}

	done := make(chan struct{})
	server := &http.Server{Handler: http.HandlerFunc(Handle)}

	if *exitAfter != 0 {
		go func() {
			d := time.Duration(*exitAfter) * time.Second
			t := time.NewTimer(d)
			for {
				select {
				case <-pleaseQuit:
					log.Printf("will exit after %v\n", d)
					t.Reset(d)
				case <-t.C:
					log.Println("shutting down")
					ctx, cancel := context.WithTimeout(context.Background(),
						2*time.Second)
					defer cancel()
					server.SetKeepAlivesEnabled(false)
					if err := server.Shutdown(ctx); err != nil {
						log.Panicf("cannot gracefully shut down: %s", err)
					}
					close(done)
					return
				}
			}
		}()
	}

	server.Serve(listeners[0])
	<-done
}
