// Package apiclient is a thin HTTP wrapper around the Aranea REST API
// (/api/v1/*). Every Cobra sub-command and the console launcher route
// their requests through this client so that authentication, output
// format defaults, and error reporting stay consistent.
package apiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	cliconfig "arenea/backend/cmd/aranea/internal/config"
)

// GlobalContext carries flags and resolved configuration that are shared
// between every sub-command. A pointer to a single instance is wired into
// the Cobra tree by root.New() and consulted lazily by each command.
type GlobalContext struct {
	BaseURL string
	Token   string
	Output  string
	Quiet   bool
	Yes     bool
	Profile string
	NoColor bool
	Timeout time.Duration

	// Resolved configuration loaded from ~/.aranea/config.toml plus
	// environment variables. Filled in by Resolve().
	Config *cliconfig.Config

	resolved bool
	client   *Client
}

// NewGlobalContext returns a zero-valued context. The caller is expected
// to populate the public fields from Cobra flags and call Resolve()
// inside PersistentPreRunE.
func NewGlobalContext() *GlobalContext {
	return &GlobalContext{}
}

// Resolve loads configuration from disk (if present) and merges
// environment variables and CLI flags following the precedence defined
// in 前端/25 cli.md §10:
//
//	flag > environment variable > active profile > defaults.
func (g *GlobalContext) Resolve() error {
	if g.resolved {
		return nil
	}
	cfg, err := cliconfig.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	profile := g.Profile
	if profile == "" {
		profile = os.Getenv("ARANEA_PROFILE")
	}
	if profile == "" {
		profile = cfg.Default
	}
	if profile == "" {
		profile = "default"
	}
	prof := cfg.Profile(profile)

	if g.BaseURL == "" {
		if env := os.Getenv("ARANEA_BASE_URL"); env != "" {
			g.BaseURL = env
		} else if prof != nil && prof.BaseURL != "" {
			g.BaseURL = prof.BaseURL
		} else {
			g.BaseURL = "http://127.0.0.1:8787"
		}
	}
	if g.Token == "" {
		if env := os.Getenv("ARANEA_TOKEN"); env != "" {
			g.Token = env
		} else if prof != nil {
			g.Token = prof.Token
		}
	}
	if g.Output == "" {
		if env := os.Getenv("ARANEA_OUTPUT"); env != "" {
			g.Output = env
		} else if prof != nil && prof.Output != "" {
			g.Output = prof.Output
		} else {
			g.Output = "text"
		}
	}
	if g.Timeout == 0 {
		g.Timeout = 60 * time.Second
	}

	g.Config = cfg
	g.client = newClient(g.BaseURL, g.Token, g.Timeout)
	g.resolved = true
	return nil
}

// Client returns the lazily constructed HTTP client. Resolve() must have
// been called first; in practice that happens automatically through the
// PersistentPreRunE hook on the root command.
func (g *GlobalContext) Client() *Client {
	if g.client == nil {
		// Defensive: build a default client so commands invoked without
		// the Cobra framework (tests, console launcher) still work.
		_ = g.Resolve()
	}
	return g.client
}

// Client is a small wrapper around *http.Client that injects the base
// URL, the bearer token and a couple of convenience helpers. It
// intentionally does NOT generate typed bindings: each sub-command marks
// the response shape it expects.
type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

func newClient(baseURL, token string, timeout time.Duration) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		http:    &http.Client{Timeout: timeout},
	}
}

// BaseURL exposes the resolved backend root, mostly for the console
// launcher which needs to construct streaming URLs by hand.
func (c *Client) BaseURL() string { return c.baseURL }

// Token exposes the resolved bearer token, used by the streaming helpers
// in the console launcher to attach Authorization headers.
func (c *Client) Token() string { return c.token }

// Get performs a GET request against the API and decodes the JSON
// response into out. Pass nil for out to ignore the body.
func (c *Client) Get(ctx context.Context, path string, query url.Values, out any) error {
	return c.do(ctx, http.MethodGet, path, query, nil, "", out)
}

// Post performs a POST request with a JSON body.
func (c *Client) Post(ctx context.Context, path string, body any, out any) error {
	return c.do(ctx, http.MethodPost, path, nil, body, "application/json", out)
}

// Put performs a PUT request with a JSON body.
func (c *Client) Put(ctx context.Context, path string, body any, out any) error {
	return c.do(ctx, http.MethodPut, path, nil, body, "application/json", out)
}

// Patch performs a PATCH request with a JSON body.
func (c *Client) Patch(ctx context.Context, path string, body any, out any) error {
	return c.do(ctx, http.MethodPatch, path, nil, body, "application/json", out)
}

// Delete performs a DELETE request, optionally with a JSON body.
func (c *Client) Delete(ctx context.Context, path string, body any, out any) error {
	return c.do(ctx, http.MethodDelete, path, nil, body, "application/json", out)
}

// PostMultipart posts a multipart/form-data body. The first argument is
// the form field name for the file part; the second is the file name
// reported to the server.
func (c *Client) PostMultipart(ctx context.Context, path, fieldName, fileName string, body io.Reader, extraFields map[string]string, out any) error {
	pipeR, pipeW := io.Pipe()
	mw := multipart.NewWriter(pipeW)

	go func() {
		defer pipeW.Close()
		defer mw.Close()
		for k, v := range extraFields {
			if err := mw.WriteField(k, v); err != nil {
				_ = pipeW.CloseWithError(err)
				return
			}
		}
		fw, err := mw.CreateFormFile(fieldName, fileName)
		if err != nil {
			_ = pipeW.CloseWithError(err)
			return
		}
		if _, err := io.Copy(fw, body); err != nil {
			_ = pipeW.CloseWithError(err)
			return
		}
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url(path, nil), pipeR)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	c.injectAuth(req)
	return c.execute(req, out)
}

func (c *Client) do(ctx context.Context, method, path string, query url.Values, body any, contentType string, out any) error {
	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode request body: %w", err)
		}
		reader = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.url(path, query), reader)
	if err != nil {
		return err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("Accept", "application/json")
	c.injectAuth(req)
	return c.execute(req, out)
}

func (c *Client) execute(req *http.Request, out any) error {
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("call %s %s: %w", req.Method, req.URL.Path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return parseAPIError(resp.StatusCode, raw)
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(out); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func (c *Client) injectAuth(req *http.Request) {
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	req.Header.Set("User-Agent", "aranea-cli")
}

func (c *Client) url(path string, query url.Values) string {
	full := c.baseURL + path
	if query != nil && len(query) > 0 {
		full += "?" + query.Encode()
	}
	return full
}

// APIError is the structured error type returned by the backend for any
// 4xx/5xx response. It implements `error` so it can be compared with
// errors.As / errors.Is upstream.
type APIError struct {
	Status  int
	Code    string `json:"code"`
	Message string `json:"message"`
	Detail  any    `json:"detail,omitempty"`
}

func (e *APIError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("api %d %s: %s", e.Status, e.Code, e.Message)
	}
	return fmt.Sprintf("api %d: %s", e.Status, e.Message)
}

func parseAPIError(status int, raw []byte) error {
	apiErr := &APIError{Status: status}
	if len(raw) == 0 {
		apiErr.Message = http.StatusText(status)
		return apiErr
	}
	var wrapper struct {
		Error *APIError `json:"error"`
	}
	if err := json.Unmarshal(raw, &wrapper); err == nil && wrapper.Error != nil {
		wrapper.Error.Status = status
		return wrapper.Error
	}
	if err := json.Unmarshal(raw, apiErr); err == nil && (apiErr.Code != "" || apiErr.Message != "") {
		return apiErr
	}
	apiErr.Message = strings.TrimSpace(string(raw))
	if apiErr.Message == "" {
		apiErr.Message = http.StatusText(status)
	}
	return apiErr
}
