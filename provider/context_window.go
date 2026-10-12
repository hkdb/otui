package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"otui/model"
	"otui/ollama"
)

// Context window discovery for OpenAI compatible servers.
//
// The OpenAI API has no standard way to report a model's context window, so
// each server exposes it differently. Probes run in order until one reports
// the context the server actually runs the model with:
//
//   - /v1/models extra fields: llama.cpp meta.n_ctx (b9076+), vLLM max_model_len
//   - llama.cpp /props: default_generation_settings.n_ctx (per slot)
//   - LM Studio /api/v1/models (0.4+) and /api/v0/models/{id} (0.3.6+)
//   - Ollama /api/ps and /api/show (Ollama's /v1 endpoint)
//
// If none report a runtime value, the trained context (llama.cpp
// meta.n_ctx_train, LM Studio max_context_length) is used.

const (
	// probeTimeout bounds a single probe request
	probeTimeout = 3 * time.Second

	// maxProbeResponseSize caps how much of a probe response is read
	maxProbeResponseSize = 8 << 20
)

// Server kinds detected by the probes, remembered so later refreshes go
// straight to the probe that worked
const (
	serverKindUnknown  = ""
	serverKindModels   = "v1-models" // /v1/models carries the runtime context (llama.cpp b9076+, vLLM)
	serverKindLlamaCpp = "llama.cpp"
	serverKindLMStudio = "lmstudio"
	serverKindOllama   = "ollama"
)

// contextProber discovers the context window of models on an OpenAI
// compatible server
type contextProber struct {
	baseURL string // e.g. http://localhost:8080/v1
	rootURL string // baseURL without the trailing /v1
	apiKey  string
	client  *http.Client

	mu   sync.Mutex
	kind string // detected server kind
}

func newContextProber(baseURL, apiKey string) *contextProber {
	base := strings.TrimRight(baseURL, "/")
	root := strings.TrimSuffix(base, "/v1")

	return &contextProber{
		baseURL: base,
		rootURL: root,
		apiKey:  apiKey,
		client:  &http.Client{Timeout: probeTimeout},
	}
}

// probeResult is what a probe found out about a model
type probeResult struct {
	runtime int    // Context the server runs the model with
	trained int    // Model's trained/max context
	source  string // Probe that found runtime (or trained)
}

// contextWindow returns the context window for modelName and where it came
// from, or 0 if no probe could tell
func (cp *contextProber) contextWindow(ctx context.Context, modelName string) (int, string) {
	cp.mu.Lock()
	kind := cp.kind
	cp.mu.Unlock()

	probes := []struct {
		kind  string
		probe func(context.Context, string) (probeResult, error)
	}{
		{serverKindModels, cp.probeModels},
		{serverKindLlamaCpp, cp.probeLlamaCppProps},
		{serverKindLMStudio, cp.probeLMStudio},
		{serverKindOllama, cp.probeOllama},
	}

	// Try the probe that worked last time first
	if kind != serverKindUnknown {
		for _, p := range probes {
			if p.kind == kind {
				if res, err := p.probe(ctx, modelName); err == nil && res.runtime > 0 {
					return res.runtime, res.source
				}
			}
		}
	}

	trained, trainedSource := 0, ""
	for _, p := range probes {
		if ctx.Err() != nil {
			break
		}
		res, err := p.probe(ctx, modelName)
		if err != nil {
			continue
		}
		if res.runtime > 0 {
			cp.mu.Lock()
			cp.kind = p.kind
			cp.mu.Unlock()
			return res.runtime, res.source
		}
		if res.trained > 0 && trained == 0 {
			trained, trainedSource = res.trained, res.source
		}
	}

	return trained, trainedSource
}

// probeModels reads context fields from /v1/models
func (cp *contextProber) probeModels(ctx context.Context, modelName string) (probeResult, error) {
	var resp struct {
		Data []struct {
			ID          string `json:"id"`
			MaxModelLen int    `json:"max_model_len"` // vLLM
			Meta        *struct {
				NCtx      int `json:"n_ctx"`       // llama.cpp b9076+ (per slot)
				NCtxTrain int `json:"n_ctx_train"` // llama.cpp
			} `json:"meta"`
		} `json:"data"`
	}
	if err := cp.getJSON(ctx, cp.baseURL+"/models", &resp); err != nil {
		return probeResult{}, err
	}

	for _, m := range resp.Data {
		if m.ID != modelName {
			continue
		}
		var res probeResult
		switch {
		case m.Meta != nil && m.Meta.NCtx > 0:
			res.runtime, res.source = m.Meta.NCtx, "llama.cpp:/v1/models meta.n_ctx"
		case m.MaxModelLen > 0:
			res.runtime, res.source = m.MaxModelLen, "vllm:/v1/models max_model_len"
		}
		if m.Meta != nil && m.Meta.NCtxTrain > 0 {
			res.trained = m.Meta.NCtxTrain
			if res.source == "" {
				res.source = "llama.cpp:/v1/models meta.n_ctx_train"
			}
		}
		return res, nil
	}
	return probeResult{}, fmt.Errorf("model %s not in /v1/models", modelName)
}

// probeLlamaCppProps reads the per-slot context from llama.cpp's /props,
// which lives at the server root (not under /v1)
func (cp *contextProber) probeLlamaCppProps(ctx context.Context, modelName string) (probeResult, error) {
	type props struct {
		DefaultGenerationSettings struct {
			NCtx int `json:"n_ctx"`
		} `json:"default_generation_settings"`
	}

	var resp props
	err := cp.getJSON(ctx, cp.rootURL+"/props", &resp)
	if err == nil && resp.DefaultGenerationSettings.NCtx > 0 {
		return probeResult{runtime: resp.DefaultGenerationSettings.NCtx, source: "llama.cpp:/props"}, nil
	}

	// Router mode (multiple models) needs ?model=. autoload=false keeps the
	// probe from loading a model that isn't loaded.
	query := url.Values{"model": {modelName}, "autoload": {"false"}}
	resp = props{}
	if err := cp.getJSON(ctx, cp.rootURL+"/props?"+query.Encode(), &resp); err != nil {
		return probeResult{}, err
	}
	if resp.DefaultGenerationSettings.NCtx > 0 {
		return probeResult{runtime: resp.DefaultGenerationSettings.NCtx, source: "llama.cpp:/props?model"}, nil
	}
	return probeResult{}, fmt.Errorf("no n_ctx in /props")
}

// probeLMStudio reads loaded/max context from LM Studio's REST API
func (cp *contextProber) probeLMStudio(ctx context.Context, modelName string) (probeResult, error) {
	// v1 API (LM Studio 0.4+)
	var v1 struct {
		Models []struct {
			Key              string `json:"key"`
			ID               string `json:"id"`
			MaxContextLength int    `json:"max_context_length"`
			LoadedInstances  []struct {
				ID     string `json:"id"`
				Config struct {
					ContextLength int `json:"context_length"`
				} `json:"config"`
			} `json:"loaded_instances"`
		} `json:"models"`
	}
	if err := cp.getJSON(ctx, cp.rootURL+"/api/v1/models", &v1); err == nil {
		for _, m := range v1.Models {
			matches := m.Key == modelName || m.ID == modelName
			for _, inst := range m.LoadedInstances {
				if inst.ID == modelName {
					matches = true
				}
			}
			if !matches {
				continue
			}
			res := probeResult{trained: m.MaxContextLength, source: "lmstudio:/api/v1/models max_context_length"}
			for _, inst := range m.LoadedInstances {
				if inst.Config.ContextLength > 0 {
					res.runtime, res.source = inst.Config.ContextLength, "lmstudio:/api/v1/models loaded context"
					break
				}
			}
			return res, nil
		}
	}

	// v0 API (LM Studio 0.3.6+)
	var v0 struct {
		State               string `json:"state"`
		MaxContextLength    int    `json:"max_context_length"`
		LoadedContextLength int    `json:"loaded_context_length"`
	}
	if err := cp.getJSON(ctx, cp.rootURL+"/api/v0/models/"+url.PathEscape(modelName), &v0); err != nil {
		return probeResult{}, err
	}
	res := probeResult{trained: v0.MaxContextLength, source: "lmstudio:/api/v0 max_context_length"}
	if v0.LoadedContextLength > 0 {
		res.runtime, res.source = v0.LoadedContextLength, "lmstudio:/api/v0 loaded_context_length"
	}
	if res.runtime == 0 && res.trained == 0 {
		return probeResult{}, fmt.Errorf("no context in LM Studio response")
	}
	return res, nil
}

// probeOllama handles Ollama's OpenAI compatible /v1 endpoint, which doesn't
// report context; Ollama's native API on the same host does
func (cp *contextProber) probeOllama(ctx context.Context, modelName string) (probeResult, error) {
	client, err := ollama.NewClient(cp.rootURL, modelName, cp.apiKey)
	if err != nil {
		return probeResult{}, err
	}

	mc, err := client.GetModelContext(ctx, modelName)
	if err != nil {
		return probeResult{}, err
	}

	res := probeResult{trained: mc.Trained, source: "ollama:model_info"}
	if mc.Loaded > 0 || mc.Modelfile > 0 {
		res.runtime, res.source = mc.Effective()
	}
	return res, nil
}

// getJSON GETs url and decodes a successful JSON response into out
func (cp *contextProber) getJSON(ctx context.Context, rawURL string, out any) error {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if cp.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+cp.apiKey)
	}

	resp, err := cp.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxProbeResponseSize))
		return fmt.Errorf("GET %s: %s", rawURL, resp.Status)
	}

	return json.NewDecoder(io.LimitReader(resp.Body, maxProbeResponseSize)).Decode(out)
}

// rawJSONInt parses an integer from a raw JSON value (as returned by the
// SDKs' respjson.Field.Raw()); returns 0 for null, empty or non-numbers
func rawJSONInt(raw string) int {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return 0
	}
	if n, err := strconv.Atoi(raw); err == nil {
		return n
	}
	if f, err := strconv.ParseFloat(raw, 64); err == nil {
		return int(f)
	}
	return 0
}

// toModelMetadata converts the fallback table type to model.ModelMetadata
func toModelMetadata(meta ModelMetadata, source string) model.ModelMetadata {
	return model.ModelMetadata{
		ContextWindow: meta.ContextWindow,
		MaxOutput:     meta.MaxOutput,
		SupportsTools: meta.SupportsTools,
		Source:        source,
	}
}
