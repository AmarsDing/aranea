package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"arenea/backend/internal/domain"
	"arenea/backend/internal/repository"
)

type PlatformService struct {
	repo repository.Store
}

func NewPlatformService(repo repository.Store) *PlatformService {
	return &PlatformService{repo: repo}
}

func (s *PlatformService) List(resource string) ([]domain.PlatformResource, error) {
	return s.repo.ListPlatformResources(resource)
}

func (s *PlatformService) Tree(resource string) ([]domain.PlatformResourceTreeNode, error) {
	items, err := s.repo.ListPlatformResources(resource)
	if err != nil {
		return nil, err
	}
	nodes := make(map[string]domain.PlatformResource, len(items))
	order := make([]string, 0, len(items))
	for _, item := range items {
		nodes[item.ID] = item
		order = append(order, item.ID)
	}

	childrenByParent := make(map[string][]string, len(items))
	rootIDs := []string{}
	for _, id := range order {
		node := nodes[id]
		if node.ParentID != "" {
			if _, ok := nodes[node.ParentID]; ok {
				childrenByParent[node.ParentID] = append(childrenByParent[node.ParentID], id)
				continue
			}
		}
		rootIDs = append(rootIDs, id)
	}

	var buildNode func(id string) domain.PlatformResourceTreeNode
	buildNode = func(id string) domain.PlatformResourceTreeNode {
		node := domain.PlatformResourceTreeNode{PlatformResource: nodes[id]}
		for _, childID := range childrenByParent[id] {
			node.Children = append(node.Children, buildNode(childID))
		}
		return node
	}

	roots := make([]domain.PlatformResourceTreeNode, 0, len(rootIDs))
	for _, id := range rootIDs {
		roots = append(roots, buildNode(id))
	}
	return roots, nil
}

func (s *PlatformService) Create(resource string, in domain.PlatformResource) (domain.PlatformResource, error) {
	in.Resource = resource
	in.Key = strings.TrimSpace(in.Key)
	in.Name = strings.TrimSpace(in.Name)
	if in.Key == "" || in.Name == "" {
		return domain.PlatformResource{}, errors.New("key and name are required")
	}
	if in.ID == "" {
		in.ID = newID()
	}
	if in.Status == "" {
		in.Status = "active"
	}
	if resource == "agent-categories" {
		if err := s.normalizeAgentCategory(&in); err != nil {
			return domain.PlatformResource{}, err
		}
	}
	created, err := s.repo.CreatePlatformResource(in)
	if err != nil {
		return domain.PlatformResource{}, err
	}
	if resource == "llm-provider-models" {
		if err = s.syncProviderModelPricing(created); err != nil {
			return domain.PlatformResource{}, err
		}
	}
	return created, nil
}

func (s *PlatformService) Update(resource string, id string, in domain.PlatformResource) (domain.PlatformResource, error) {
	if id == "" {
		return domain.PlatformResource{}, errors.New("id is required")
	}
	in.ID = id
	in.Resource = resource
	if resource == "agent-categories" {
		if err := s.normalizeAgentCategory(&in); err != nil {
			return domain.PlatformResource{}, err
		}
	}
	updated, err := s.repo.UpdatePlatformResource(in)
	if err != nil {
		return domain.PlatformResource{}, err
	}
	if resource == "llm-provider-models" {
		if err = s.syncProviderModelPricing(updated); err != nil {
			return domain.PlatformResource{}, err
		}
	}
	return updated, nil
}

func (s *PlatformService) Delete(resource string, id string) error {
	if id == "" {
		return errors.New("id is required")
	}
	return s.repo.DeletePlatformResource(resource, id)
}

func (s *PlatformService) ListAvatarAssets(scope string, workspaceID string, ownerUserID string) ([]domain.AvatarAsset, error) {
	return s.repo.ListAvatarAssets(scope, workspaceID, ownerUserID)
}

func (s *PlatformService) GetAvatarImage(id string, thumbnail bool) (domain.AvatarImage, error) {
	return s.repo.GetAvatarImage(id, thumbnail)
}

func (s *PlatformService) UploadAvatar(file multipart.File, header *multipart.FileHeader, workspaceID string, ownerUserID string) (domain.AvatarAsset, error) {
	if file == nil || header == nil {
		return domain.AvatarAsset{}, errors.New("avatar file is required")
	}
	if header.Size > 2*1024*1024 {
		return domain.AvatarAsset{}, errors.New("avatar file must be <= 2MB")
	}
	data, err := io.ReadAll(io.LimitReader(file, 2*1024*1024+1))
	if err != nil {
		return domain.AvatarAsset{}, err
	}
	if len(data) > 2*1024*1024 {
		return domain.AvatarAsset{}, errors.New("avatar file must be <= 2MB")
	}
	mimeType := http.DetectContentType(data)
	if mimeType != "image/png" && mimeType != "image/jpeg" && mimeType != "image/webp" {
		return domain.AvatarAsset{}, fmt.Errorf("unsupported avatar type: %s", mimeType)
	}
	id := newID()
	asset := domain.AvatarAsset{
		ID:            id,
		Key:           "upload-" + id,
		Name:          strings.TrimSpace(header.Filename),
		Description:   "用户上传头像",
		MimeType:      mimeType,
		WorkspaceID:   workspaceID,
		OwnerUserID:   ownerUserID,
		Source:        "upload",
		IsSystem:      false,
		FileSizeBytes: len(data),
		WidthPx:       0,
		HeightPx:      0,
		SortOrder:     1000,
	}
	if asset.Name == "" {
		asset.Name = "上传头像"
	}
	return s.repo.CreateAvatarAsset(asset, data, data)
}

func (s *PlatformService) ValidateModel(in domain.ValidateModelInput) (domain.ValidateModelResult, error) {
	ok, err := s.repo.ValidateProviderModel(strings.TrimSpace(in.Provider), strings.TrimSpace(in.Model))
	if err != nil {
		return domain.ValidateModelResult{}, err
	}
	if ok {
		return domain.ValidateModelResult{OK: true, Message: "model is available"}, nil
	}
	return domain.ValidateModelResult{OK: false, Message: "provider/model is not enabled"}, nil
}

func (s *PlatformService) InspectProviderModel(in domain.InspectProviderModelInput) (domain.InspectProviderModelResult, error) {
	in.ResourceID = strings.TrimSpace(in.ResourceID)
	in.ProviderCode = strings.TrimSpace(in.ProviderCode)
	in.ProviderType = strings.TrimSpace(in.ProviderType)
	in.ModelAPIID = strings.TrimSpace(in.ModelAPIID)
	in.APIBaseURL = strings.TrimSpace(in.APIBaseURL)
	in.APIKey = strings.TrimSpace(in.APIKey)
	if in.ProviderCode == "" || in.ModelAPIID == "" {
		return domain.InspectProviderModelResult{}, errors.New("provider_code and model_api_id are required")
	}
	if err := s.fillInspectConnectionFromSavedConfig(&in); err != nil {
		return domain.InspectProviderModelResult{}, err
	}
	if strings.Contains(strings.ToLower(in.APIBaseURL), "openrouter.ai") || strings.Contains(strings.ToLower(in.ProviderCode), "openrouter") {
		return inspectOpenRouterModel(in)
	}
	if strings.Contains(strings.ToLower(in.ProviderType), "anthropic") {
		return inspectAnthropicModel(in)
	}
	return inspectOpenAICompatibleModel(in)
}

func (s *PlatformService) fillInspectConnectionFromSavedConfig(in *domain.InspectProviderModelInput) error {
	if in.APIBaseURL != "" && in.APIKey != "" && in.ProviderType != "" {
		return nil
	}
	var row domain.PlatformResource
	var err error
	if in.ResourceID != "" {
		row, err = s.repo.GetPlatformResource("llm-provider-models", in.ResourceID)
	} else {
		row, err = s.repo.GetProviderModel(in.ProviderCode, in.ModelAPIID)
	}
	if err != nil {
		return nil
	}
	var cfg struct {
		ProviderType string `json:"provider_type"`
		APIBaseURL   string `json:"api_base_url"`
		APIKey       string `json:"api_key"`
	}
	if json.Unmarshal([]byte(row.ConfigJSON), &cfg) != nil {
		return nil
	}
	if in.ProviderType == "" {
		in.ProviderType = cfg.ProviderType
	}
	if in.APIBaseURL == "" {
		in.APIBaseURL = cfg.APIBaseURL
	}
	if in.APIKey == "" {
		in.APIKey = cfg.APIKey
	}
	return nil
}

type providerPricingConfig struct {
	InputPriceMicroUSDPer1K       int64 `json:"input_price_micro_usd_per_1k"`
	OutputPriceMicroUSDPer1K      int64 `json:"output_price_micro_usd_per_1k"`
	CachedInputPriceMicroUSDPer1K int64 `json:"cached_input_price_micro_usd_per_1k"`
	ReasoningPriceMicroUSDPer1K   int64 `json:"reasoning_price_micro_usd_per_1k"`
	EmbeddingPriceMicroUSDPer1K   int64 `json:"embedding_price_micro_usd_per_1k"`
}

func (s *PlatformService) syncProviderModelPricing(row domain.PlatformResource) error {
	var cfg providerPricingConfig
	if json.Unmarshal([]byte(row.ConfigJSON), &cfg) != nil {
		return nil
	}
	if cfg.InputPriceMicroUSDPer1K == 0 && cfg.OutputPriceMicroUSDPer1K == 0 && cfg.CachedInputPriceMicroUSDPer1K == 0 && cfg.ReasoningPriceMicroUSDPer1K == 0 && cfg.EmbeddingPriceMicroUSDPer1K == 0 {
		return nil
	}
	_, err := s.repo.UpsertModelPricingRule(domain.ModelPricingRule{
		ProviderCode:                  row.Provider,
		ModelAPIID:                    row.Model,
		Currency:                      "USD",
		InputPriceMicroUSDPer1K:       cfg.InputPriceMicroUSDPer1K,
		OutputPriceMicroUSDPer1K:      cfg.OutputPriceMicroUSDPer1K,
		CachedInputPriceMicroUSDPer1K: cfg.CachedInputPriceMicroUSDPer1K,
		ReasoningPriceMicroUSDPer1K:   cfg.ReasoningPriceMicroUSDPer1K,
		EmbeddingPriceMicroUSDPer1K:   cfg.EmbeddingPriceMicroUSDPer1K,
		Source:                        "model-inspect",
		MetadataJSON:                  "{}",
	})
	return err
}

func inspectOpenRouterModel(in domain.InspectProviderModelInput) (domain.InspectProviderModelResult, error) {
	endpoint := "https://openrouter.ai/api/v1/models"
	if in.APIBaseURL != "" {
		endpoint = openRouterModelsURL(in.APIBaseURL)
	}
	var out struct {
		Data []struct {
			ID            string `json:"id"`
			Name          string `json:"name"`
			ContextLength int    `json:"context_length"`
			Pricing       struct {
				Prompt     string `json:"prompt"`
				Completion string `json:"completion"`
			} `json:"pricing"`
			TopProvider struct {
				MaxCompletionTokens int `json:"max_completion_tokens"`
			} `json:"top_provider"`
		} `json:"data"`
	}
	if err := getProviderJSON(endpoint, in.APIKey, nil, &out); err != nil {
		return domain.InspectProviderModelResult{OK: false, Message: "OpenRouter 模型参数请求失败：" + err.Error(), ProviderCode: in.ProviderCode, ProviderType: in.ProviderType, ModelAPIID: in.ModelAPIID}, nil
	}
	for _, item := range out.Data {
		if item.ID != in.ModelAPIID {
			continue
		}
		raw, _ := json.Marshal(item)
		return domain.InspectProviderModelResult{
			OK:                       true,
			Message:                  "已从 OpenRouter 获取模型参数",
			ProviderCode:             in.ProviderCode,
			ProviderType:             firstNonEmpty(in.ProviderType, "OpenAI Compatible"),
			ModelAPIID:               item.ID,
			ModelDisplayName:         firstNonEmpty(item.Name, item.ID),
			ModelSizeLabel:           inferModelSizeLabel(item.ID + " " + item.Name),
			ContextWindowK:           tokensToK(item.ContextLength),
			MaxOutputTokens:          item.TopProvider.MaxCompletionTokens,
			InputPriceMicroUSDPer1K:  priceStringToMicroUSDPer1K(item.Pricing.Prompt),
			OutputPriceMicroUSDPer1K: priceStringToMicroUSDPer1K(item.Pricing.Completion),
			Source:                   "openrouter",
			RawMetadataJSON:          string(raw),
		}, nil
	}
	return domain.InspectProviderModelResult{OK: false, Message: "OpenRouter 未找到该模型", ProviderCode: in.ProviderCode, ProviderType: in.ProviderType, ModelAPIID: in.ModelAPIID}, nil
}

func inspectOpenAICompatibleModel(in domain.InspectProviderModelInput) (domain.InspectProviderModelResult, error) {
	if in.APIBaseURL == "" {
		return domain.InspectProviderModelResult{OK: false, Message: "检查模型需要 API 基础 URL", ProviderCode: in.ProviderCode, ProviderType: in.ProviderType, ModelAPIID: in.ModelAPIID}, nil
	}
	var out struct {
		Data []struct {
			ID      string `json:"id"`
			Object  string `json:"object"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}
	if err := getProviderJSON(modelsURL(in.APIBaseURL), in.APIKey, nil, &out); err != nil {
		return domain.InspectProviderModelResult{OK: false, Message: "Provider 模型参数请求失败：" + err.Error(), ProviderCode: in.ProviderCode, ProviderType: in.ProviderType, ModelAPIID: in.ModelAPIID}, nil
	}
	for _, item := range out.Data {
		if item.ID == in.ModelAPIID {
			raw, _ := json.Marshal(item)
			return domain.InspectProviderModelResult{
				OK:               true,
				Message:          "已验证模型存在；该 Provider 未返回上下文和价格",
				ProviderCode:     in.ProviderCode,
				ProviderType:     firstNonEmpty(in.ProviderType, "OpenAI Compatible"),
				ModelAPIID:       item.ID,
				ModelDisplayName: item.ID,
				ModelSizeLabel:   inferModelSizeLabel(item.ID),
				Source:           "openai-compatible",
				RawMetadataJSON:  string(raw),
			}, nil
		}
	}
	return domain.InspectProviderModelResult{OK: false, Message: "Provider /models 未找到该模型", ProviderCode: in.ProviderCode, ProviderType: in.ProviderType, ModelAPIID: in.ModelAPIID}, nil
}

func inspectAnthropicModel(in domain.InspectProviderModelInput) (domain.InspectProviderModelResult, error) {
	base := firstNonEmpty(in.APIBaseURL, "https://api.anthropic.com/v1")
	var out struct {
		Data []struct {
			ID          string `json:"id"`
			DisplayName string `json:"display_name"`
		} `json:"data"`
	}
	headers := map[string]string{"anthropic-version": "2023-06-01"}
	if err := getProviderJSON(modelsURL(base), in.APIKey, headers, &out); err != nil {
		return anthropicKnownModelFallback(in, "Anthropic 元数据接口不可用，已根据模型ID使用内置参数："+err.Error()), nil
	}
	for _, item := range out.Data {
		if item.ID == in.ModelAPIID {
			raw, _ := json.Marshal(item)
			fallback := anthropicKnownModelDefaults(in)
			return domain.InspectProviderModelResult{
				OK:                       true,
				Message:                  "已从 Anthropic 获取模型名称；价格和上下文需手动维护",
				ProviderCode:             in.ProviderCode,
				ProviderType:             firstNonEmpty(in.ProviderType, "Anthropic"),
				ModelAPIID:               item.ID,
				ModelDisplayName:         firstNonEmpty(item.DisplayName, item.ID),
				ModelSizeLabel:           inferModelSizeLabel(item.ID + " " + item.DisplayName),
				ContextWindowK:           fallback.ContextWindowK,
				MaxOutputTokens:          fallback.MaxOutputTokens,
				InputPriceMicroUSDPer1K:  fallback.InputPriceMicroUSDPer1K,
				OutputPriceMicroUSDPer1K: fallback.OutputPriceMicroUSDPer1K,
				Source:                   "anthropic",
				RawMetadataJSON:          string(raw),
			}, nil
		}
	}
	return anthropicKnownModelFallback(in, "Anthropic 元数据接口未返回该模型，已根据模型ID使用内置参数"), nil
}

func anthropicKnownModelFallback(in domain.InspectProviderModelInput, message string) domain.InspectProviderModelResult {
	result := anthropicKnownModelDefaults(in)
	result.OK = true
	result.Message = message
	result.RawMetadataJSON = fmt.Sprintf(`{"source":"anthropic-known-defaults","model":"%s"}`, in.ModelAPIID)
	return result
}

func anthropicKnownModelDefaults(in domain.InspectProviderModelInput) domain.InspectProviderModelResult {
	model := strings.ToLower(in.ModelAPIID)
	result := domain.InspectProviderModelResult{
		ProviderCode:     in.ProviderCode,
		ProviderType:     firstNonEmpty(in.ProviderType, "Anthropic"),
		ModelAPIID:       in.ModelAPIID,
		ModelDisplayName: in.ModelAPIID,
		ContextWindowK:   200,
		MaxOutputTokens:  8192,
		Source:           "anthropic-known-defaults",
	}
	switch {
	case strings.Contains(model, "opus"):
		result.InputPriceMicroUSDPer1K = 15000
		result.OutputPriceMicroUSDPer1K = 75000
	case strings.Contains(model, "haiku"):
		result.InputPriceMicroUSDPer1K = 800
		result.OutputPriceMicroUSDPer1K = 4000
	default:
		result.InputPriceMicroUSDPer1K = 3000
		result.OutputPriceMicroUSDPer1K = 15000
	}
	return result
}

func getProviderJSON(endpoint string, apiKey string, headers map[string]string, out any) error {
	client := &http.Client{Timeout: 15 * time.Second}
	request, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	if apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+apiKey)
		request.Header.Set("x-api-key", apiKey)
	}
	request.Header.Set("Accept", "application/json")
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 2048))
		return fmt.Errorf("provider metadata request failed: %s %s", response.Status, strings.TrimSpace(string(body)))
	}
	return json.NewDecoder(response.Body).Decode(out)
}

func openRouterModelsURL(base string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(base), "/")
	if strings.Contains(trimmed, "/models") {
		return trimmed
	}
	if strings.Contains(trimmed, "/api/v1") {
		return strings.TrimRight(strings.Split(trimmed, "/api/v1")[0], "/") + "/api/v1/models"
	}
	return "https://openrouter.ai/api/v1/models"
}

func modelsURL(base string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(base), "/")
	if strings.HasSuffix(trimmed, "/chat/completions") {
		return strings.TrimSuffix(trimmed, "/chat/completions") + "/models"
	}
	if strings.HasSuffix(trimmed, "/messages") {
		return strings.TrimSuffix(trimmed, "/messages") + "/models"
	}
	if parsed, err := url.Parse(trimmed); err == nil && parsed.Scheme != "" && parsed.Host != "" {
		return trimmed + "/models"
	}
	return trimmed
}

func priceStringToMicroUSDPer1K(value string) int64 {
	numberValue, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || numberValue <= 0 {
		return 0
	}
	return int64(numberValue*1_000_000_000 + 0.5)
}

func tokensToK(tokens int) int {
	if tokens <= 0 {
		return 0
	}
	return int(float64(tokens)/1000 + 0.5)
}

func inferModelSizeLabel(value string) string {
	re := regexp.MustCompile(`(?i)(\d+(?:\.\d+)?\s*x\s*)?\d+(?:\.\d+)?\s*b`)
	match := re.FindString(value)
	if match == "" {
		return ""
	}
	return strings.ToUpper(strings.ReplaceAll(match, " ", ""))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func (s *PlatformService) normalizeAgentCategory(in *domain.PlatformResource) error {
	if strings.TrimSpace(in.ParentID) == "" {
		in.ParentID = ""
		if in.Level == "" {
			in.Level = "industry"
		}
		if in.Level != "industry" {
			return errors.New("industry category must not have parent_id")
		}
		return nil
	}
	parent, err := s.repo.GetPlatformResource("agent-categories", in.ParentID)
	if err != nil {
		return fmt.Errorf("parent category not found: %w", err)
	}
	switch parent.Level {
	case "industry":
		if in.Level == "" {
			in.Level = "department"
		}
		if in.Level != "department" {
			return errors.New("industry children must be department")
		}
	case "department":
		if in.Level == "" {
			in.Level = "position"
		}
		if in.Level != "position" {
			return errors.New("department children must be position")
		}
	case "position":
		return errors.New("position category cannot have children")
	default:
		return errors.New("parent category level is invalid")
	}
	return nil
}
