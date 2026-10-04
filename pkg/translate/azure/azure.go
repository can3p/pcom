// Package azure is the Azure Translator backend (TRANSLATION_PROVIDER=azure):
// the HTTP call to its v3 translate API and the mapping to and from its wire
// format. Importing the package registers it.
package azure

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/can3p/pcom/pkg/config"
	"github.com/can3p/pcom/pkg/translate"
)

// Name is the provider name in settings and in stored translations.
const Name = "azure"

func init() {
	translate.Register(Name, func(cfg config.Translation) (translate.Backend, error) {
		return New(cfg.Azure, nil), nil
	})
}

// Backend calls Azure Translator.
type Backend struct {
	endpoint string
	key      string
	region   string
	client   *http.Client
}

// New returns the backend for cfg; client nil is a client with a 30s timeout.
func New(cfg config.AzureTranslator, client *http.Client) *Backend {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	return &Backend{
		endpoint: strings.TrimRight(cfg.Endpoint, "/"),
		key:      cfg.Key.Reveal(),
		region:   cfg.Region,
		client:   client,
	}
}

func (*Backend) Name() string        { return Name }
func (*Backend) DisplayName() string { return "Azure Translator" }

// Limits are Azure's per-request limits: 1,000 elements and 50,000
// characters in all.
func (*Backend) Limits() translate.Limits {
	return translate.Limits{Segments: 1000, Chars: 50000}
}

type element struct {
	Text string `json:"Text"`
}

type result struct {
	Translations []struct {
		Text string `json:"text"`
		To   string `json:"to"`
	} `json:"translations"`
}

type errorBody struct {
	Error struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// Translate sends the segments as HTML and returns Azure's translations in
// order. Every non-2xx answer is a *translate.StatusError.
func (b *Backend) Translate(ctx context.Context, req translate.Request) ([]string, error) {
	in := make([]element, len(req.Segments))
	for i, s := range req.Segments {
		in[i] = element{Text: s}
	}

	body, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}

	q := url.Values{"api-version": {"3.0"}, "to": {req.To}, "textType": {"html"}}
	if req.From != "" {
		q.Set("from", req.From)
	}

	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, b.endpoint+"/translate?"+q.Encode(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	hreq.Header.Set("Content-Type", "application/json; charset=UTF-8")
	if b.key != "" { // config.Translation.Validate requires one; Azure answers 401 without
		hreq.Header.Set("Ocp-Apim-Subscription-Key", b.key)
	}

	if b.region != "" {
		hreq.Header.Set("Ocp-Apim-Subscription-Region", b.region)
	}

	resp, err := b.client.Do(hreq)
	if err != nil {
		return nil, fmt.Errorf("azure translator: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, fmt.Errorf("azure translator: reading the response: %w", err)
	}

	if resp.StatusCode/100 != 2 {
		return nil, statusError(resp, raw)
	}

	var out []result
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("azure translator: decoding the response: %w", err)
	}

	if len(out) != len(req.Segments) {
		return nil, fmt.Errorf("azure translator: %d results for %d elements", len(out), len(req.Segments))
	}

	texts := make([]string, len(out))
	for i, r := range out {
		if len(r.Translations) != 1 {
			return nil, fmt.Errorf("azure translator: element %d has %d translations, want 1", i, len(r.Translations))
		}

		texts[i] = r.Translations[0].Text
	}

	return texts, nil
}

func statusError(resp *http.Response, raw []byte) *translate.StatusError {
	se := &translate.StatusError{Status: resp.StatusCode}

	var eb errorBody
	if json.Unmarshal(raw, &eb) == nil && eb.Error.Code != 0 {
		se.Code = strconv.Itoa(eb.Error.Code)
		se.Message = eb.Error.Message
	}

	if secs, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && secs > 0 {
		se.RetryAfter = time.Duration(secs) * time.Second
	}

	return se
}
