package webhook

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"

	log "github.com/sirupsen/logrus"
	"github.com/waldner/external-dns-he-webhook/pkg/config"
	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"
)

type Webhook interface {
	Negotiate(w http.ResponseWriter, r *http.Request)
	Records(w http.ResponseWriter, r *http.Request)
	AdjustEndpoints(w http.ResponseWriter, r *http.Request)
	ApplyChanges(w http.ResponseWriter, r *http.Request)
}

type heWebhook struct {
	Conf   *config.Config
	Client *http.Client
}

const (
	contentTypeValue = "application/external.dns.webhook+json;version=1"
)

func NewWebhook(conf *config.Config) (Webhook, error) {

	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("NewWebHook: error creating cookie jar: %s", err)
	}

	client := &http.Client{
		Jar: jar,
	}

	return &heWebhook{conf, client}, nil
}

func Health(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// return domain filter
func (h *heWebhook) Negotiate(w http.ResponseWriter, r *http.Request) {

	log.Debugf("******************* Received request in Negotiate: %+v", r)
	err := checkHeader(r, "Accept")
	if err != nil {
		log.Error("Negotiate: %s", err)
		return
	}

	df, err := h.Conf.DomainFilter.MarshalJSON()
	if err != nil {
		log.Errorf("Negotiate: failed to marshal domain filter, request method: %s, request path: %s, error: %s", r.Method, r.URL.Path, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", contentTypeValue)
	log.Debugf("Writing '%s'", df)
	if _, err := w.Write(df); err != nil {
		log.Errorf("Negotiate: error writing response: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

}

// GET to /records
// return all records matching the domain filters
func (h *heWebhook) Records(w http.ResponseWriter, r *http.Request) {

	log.Debugf("******************** Received request in Records: %+v", r)
	err := checkHeader(r, "Accept")
	if err != nil {
		log.Error("Records: %s", err)
		return
	}

	endpoints, err := getAllRecords(h)
	if err != nil {
		log.Errorf("Records: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	log.Infof("Found %d records", len(endpoints))
	w.Header().Set("Content-Type", contentTypeValue)
	w.Header().Set("Vary", "Content-Type")
	err = json.NewEncoder(w).Encode(endpoints)
	if err != nil {
		log.Errorf("Records: error during JSON encoding of records: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

}

// POST to /adjustendpoints
// IIUC we can just return the same list here
func (h *heWebhook) AdjustEndpoints(w http.ResponseWriter, r *http.Request) {

	log.Debugf("******************** Received request in AdjustEndpoints: %+v", r)
	err := checkHeader(r, "Accept")
	if err != nil {
		log.Error("AdjustEndpoints: %s", err)
		return
	}
	err = checkHeader(r, "Content-Type")
	if err != nil {
		log.Error("AdjustEndpoints: %s", err)
		return
	}

	endpoints := []*endpoint.Endpoint{}
	if err := json.NewDecoder(r.Body).Decode(&endpoints); err != nil {

		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusBadRequest)

		log.Errorf("AdjustEndpoints: error decoding request body JSON: %s", err)

		if _, err = fmt.Fprint(w, err); err != nil {
			log.Errorf("AdjustEndpoints: error writing error message to response writer: %s", err)
		}
		return
	}

	log.Debugf("Json decoding successful, endpoints are %d: %+v", len(endpoints), endpoints)

	endpoints = adjustEndpoints(endpoints)
	out, err := json.Marshal(&endpoints)
	if err != nil {
		log.Errorf("AdjustEndpoints: error marshaling return values: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", contentTypeValue)
	w.Header().Set("Vary", "Content-Type")
	if _, err = fmt.Fprint(w, string(out)); err != nil {
		log.Errorf("AdjustEndpoints: error writing response: %s", err)
	}
}

// POST to /records
func (h *heWebhook) ApplyChanges(w http.ResponseWriter, r *http.Request) {

	log.Debugf("******************** Received request in ApplyChanges: %+v", r)
	err := checkHeader(r, "Content-Type")
	if err != nil {
		log.Errorf("ApplyChanges: %s", err)
		return
	}

	// decode requested changes
	changes := plan.Changes{}
	if err := json.NewDecoder(r.Body).Decode(&changes); err != nil {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusBadRequest)
		errMsg := fmt.Sprintf("error decoding changes: %s", err.Error())
		if _, err := fmt.Fprint(w, errMsg); err != nil {
			log.Errorf("ApplyChanges: error writing response message: %s", err)
		}
		log.Warnf(errMsg)
		return
	}

	err = applyChanges(&changes, h)
	if err != nil {
		log.Errorf("ApplyChanges: %s", err)
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// check that the given header is "application/external.dns.webhook+json;version=1"
func checkHeader(r *http.Request, headerName string) error {

	headerValue := r.Header[headerName]
	if len(headerValue) == 1 {
		if headerValue[0] == contentTypeValue {
			log.Debugf("'%s' header is present, value is %s", headerName, headerValue[0])
			return nil
		} else {
			return fmt.Errorf("checkHeader: '%s' header does not have the right value: %s", headerName, headerValue[0])
		}
	}

	return fmt.Errorf("checkHeader: '%s' header not present or present more than once", headerName)
}
