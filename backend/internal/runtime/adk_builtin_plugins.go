package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/model"
	"google.golang.org/adk/plugin"
	"google.golang.org/adk/plugin/retryandreflect"
	"google.golang.org/adk/session"
	"google.golang.org/adk/tool"
	"google.golang.org/genai"
)

type builtinPluginDefinition struct {
	Key            string
	Name           string
	Description    string
	Category       string
	RiskLevel      string
	CallbackPoints []string
	Factory        func(map[string]any) (*plugin.Plugin, error)
}

type PluginRuntimeConfig struct {
	Key        string
	ConfigJSON string
}

type ConfiguredPluginSource interface {
	EnabledPluginConfigs(context.Context) ([]PluginRuntimeConfig, error)
}

type BuiltinPluginDefinition struct {
	Key               string
	Name              string
	Description       string
	Category          string
	RiskLevel         string
	CallbackPoints    []string
	DefaultConfigJSON string
	ConfigSchemaJSON  string
	SortOrder         int
}

var builtinPluginRegistry = map[string]builtinPluginDefinition{
	"runtime_audit": {
		Key:         "runtime_audit",
		Name:        "Runtime Audit",
		Description: "记录脱敏后的 Agent、模型、事件和工具执行摘要，用于运行审计。",
		Category:    "observability",
		RiskLevel:   "low",
		CallbackPoints: []string{
			"on_user_message", "before_model", "after_model", "before_tool", "after_tool", "on_tool_error", "on_event",
		},
		Factory: func(_ map[string]any) (*plugin.Plugin, error) { return newRuntimeAuditPlugin() },
	},
	"sensitive_data_mask": {
		Key:            "sensitive_data_mask",
		Name:           "Sensitive Data Mask",
		Description:    "在模型调用前后遮蔽密钥、隐私和敏感数据。",
		Category:       "guard",
		RiskLevel:      "medium",
		CallbackPoints: []string{"on_user_message", "before_model", "after_model"},
		Factory:        func(_ map[string]any) (*plugin.Plugin, error) { return newSensitiveDataMaskPlugin() },
	},
	"confirmation_guard": {
		Key:            "confirmation_guard",
		Name:           "Confirmation Guard",
		Description:    "拦截高风险工具调用，等待后续确认流程接入。",
		Category:       "guard",
		RiskLevel:      "high",
		CallbackPoints: []string{"before_tool"},
		Factory:        func(_ map[string]any) (*plugin.Plugin, error) { return newConfirmationGuardPlugin() },
	},
	"skill_usage_tracker": {
		Key:            "skill_usage_tracker",
		Name:           "Skill Usage Tracker",
		Description:    "统计 Skill 工具的成功、失败、耗时和 Agent 使用摘要。",
		Category:       "tracking",
		RiskLevel:      "low",
		CallbackPoints: []string{"before_tool", "after_tool", "on_tool_error"},
		Factory:        func(_ map[string]any) (*plugin.Plugin, error) { return newSkillUsageTrackerPlugin() },
	},
	"retry_and_reflect": {
		Key:            "retry_and_reflect",
		Name:           "Retry and Reflect",
		Description:    "对可恢复的工具失败触发 ADK 重试与反思流程。",
		Category:       "debug",
		RiskLevel:      "medium",
		CallbackPoints: []string{"on_tool_error"},
		Factory: func(_ map[string]any) (*plugin.Plugin, error) {
			return retryandreflect.New()
		},
	},
	"permission_guard": {
		Key:            "permission_guard",
		Name:           "Permission Guard",
		Description:    "在工具执行前检查权限规则，阻止未授权调用。",
		Category:       "guard",
		RiskLevel:      "high",
		CallbackPoints: []string{"before_tool"},
		Factory:        func(config map[string]any) (*plugin.Plugin, error) { return newPermissionGuardPluginWithConfig(config) },
	},
	"output_policy": {
		Key:            "output_policy",
		Name:           "Output Policy",
		Description:    "拦截危险命令、泄露密钥和违反策略的模型输出。",
		Category:       "policy",
		RiskLevel:      "high",
		CallbackPoints: []string{"after_model"},
		Factory:        func(config map[string]any) (*plugin.Plugin, error) { return newOutputPolicyPluginWithConfig(config) },
	},
	"cost_guard": {
		Key:            "cost_guard",
		Name:           "Cost Guard",
		Description:    "按 token 预算、禁用高价模型和回退路由控制模型成本。",
		Category:       "guard",
		RiskLevel:      "medium",
		CallbackPoints: []string{"before_model"},
		Factory:        func(config map[string]any) (*plugin.Plugin, error) { return newCostGuardPluginWithConfig(config) },
	},
	"model_router": {
		Key:            "model_router",
		Name:           "Model Router",
		Description:    "根据 Agent、任务类型和上下文规模选择模型路由。",
		Category:       "routing",
		RiskLevel:      "medium",
		CallbackPoints: []string{"before_model"},
		Factory:        func(config map[string]any) (*plugin.Plugin, error) { return newModelRouterPluginWithConfig(config) },
	},
}

var builtinPluginAliases = map[string]string{
	"logging":             "runtime_audit",
	"loggingplugin":       "runtime_audit",
	"audit":               "runtime_audit",
	"redaction":           "sensitive_data_mask",
	"mask":                "sensitive_data_mask",
	"confirmation":        "confirmation_guard",
	"confirm":             "confirmation_guard",
	"skill_usage":         "skill_usage_tracker",
	"skill_usage_tracker": "skill_usage_tracker",
	"retry":               "retry_and_reflect",
	"retryandreflect":     "retry_and_reflect",
	"retry_and_reflect":   "retry_and_reflect",
	"cost":                "cost_guard",
	"cost_guard":          "cost_guard",
	"router":              "model_router",
	"model_router":        "model_router",
	"permission":          "permission_guard",
	"permission_guard":    "permission_guard",
	"output":              "output_policy",
	"output_policy":       "output_policy",
}

func BuiltinPluginDefinitions() []BuiltinPluginDefinition {
	keys := make([]string, 0, len(builtinPluginRegistry))
	for key := range builtinPluginRegistry {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]BuiltinPluginDefinition, 0, len(keys))
	for index, key := range keys {
		def := builtinPluginRegistry[key]
		result = append(result, BuiltinPluginDefinition{
			Key:               def.Key,
			Name:              def.Name,
			Description:       def.Description,
			Category:          def.Category,
			RiskLevel:         def.RiskLevel,
			CallbackPoints:    append([]string{}, def.CallbackPoints...),
			DefaultConfigJSON: "{}",
			ConfigSchemaJSON:  "{}",
			SortOrder:         (index + 1) * 10,
		})
	}
	return result
}

func (a *ADKRuntimeAdapter) builtinPlugins(ctx context.Context) ([]*plugin.Plugin, error) {
	if a.pluginSource != nil {
		if source, ok := a.pluginSource.(ConfiguredPluginSource); ok {
			configs, err := source.EnabledPluginConfigs(ctx)
			if err != nil {
				return nil, err
			}
			return builtinPluginsFromConfigs(configs)
		}
		keys, err := a.pluginSource.EnabledPluginKeys(ctx)
		if err != nil {
			return nil, err
		}
		return builtinPluginsFromKeys(keys)
	}
	return builtinPluginsFromEnv()
}

func builtinPluginsFromEnv() ([]*plugin.Plugin, error) {
	raw := strings.TrimSpace(os.Getenv("ADK_RUNNER_PLUGINS"))
	if raw == "" {
		return nil, nil
	}
	keys, err := normalizeBuiltinPluginKeys(raw)
	if err != nil {
		return nil, err
	}
	return builtinPluginsFromKeys(keys)
}

func builtinPluginsFromKeys(keys []string) ([]*plugin.Plugin, error) {
	configs := make([]PluginRuntimeConfig, 0, len(keys))
	for _, key := range keys {
		configs = append(configs, PluginRuntimeConfig{Key: key})
	}
	return builtinPluginsFromConfigs(configs)
}

func builtinPluginsFromConfigs(configs []PluginRuntimeConfig) ([]*plugin.Plugin, error) {
	plugins := make([]*plugin.Plugin, 0, len(configs))
	for _, item := range configs {
		key := item.Key
		if alias, ok := builtinPluginAliases[strings.ToLower(strings.TrimSpace(key))]; ok {
			key = alias
		}
		definition, ok := builtinPluginRegistry[key]
		if !ok {
			return nil, fmt.Errorf("unsupported builtin ADK runner plugin %q", key)
		}
		p, err := definition.Factory(parsePluginConfigJSON(item.ConfigJSON))
		if err != nil {
			return nil, fmt.Errorf("create builtin ADK runner plugin %q: %w", key, err)
		}
		plugins = append(plugins, p)
	}
	return plugins, nil
}

func parsePluginConfigJSON(raw string) map[string]any {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

func normalizeBuiltinPluginKeys(raw string) ([]string, error) {
	seen := map[string]bool{}
	keys := []string{}
	for _, item := range strings.Split(raw, ",") {
		key := strings.ToLower(strings.TrimSpace(item))
		if key == "" {
			continue
		}
		if alias, ok := builtinPluginAliases[key]; ok {
			key = alias
		}
		if _, ok := builtinPluginRegistry[key]; !ok {
			available := make([]string, 0, len(builtinPluginRegistry))
			for item := range builtinPluginRegistry {
				available = append(available, item)
			}
			sort.Strings(available)
			return nil, fmt.Errorf("unsupported builtin ADK runner plugin %q, available: %s", key, strings.Join(available, ", "))
		}
		if !seen[key] {
			keys = append(keys, key)
			seen[key] = true
		}
	}
	return keys, nil
}

func newRuntimeAuditPlugin() (*plugin.Plugin, error) {
	return plugin.New(plugin.Config{
		Name: "runtime_audit",
		OnUserMessageCallback: func(_ agent.InvocationContext, content *genai.Content) (*genai.Content, error) {
			log.Printf("adk plugin runtime_audit callback=on_user_message input=%q", previewForPlugin(contentText(content), 500))
			return nil, nil
		},
		BeforeModelCallback: func(_ agent.CallbackContext, req *model.LLMRequest) (*model.LLMResponse, error) {
			log.Printf("adk plugin runtime_audit callback=before_model model=%q messages=%d input=%q", req.Model, len(req.Contents), previewForPlugin(llmRequestPreview(req), 500))
			return nil, nil
		},
		AfterModelCallback: func(_ agent.CallbackContext, resp *model.LLMResponse, responseErr error) (*model.LLMResponse, error) {
			status := "success"
			if responseErr != nil {
				status = "error"
			}
			log.Printf("adk plugin runtime_audit callback=after_model status=%s output=%q error=%v", status, previewForPlugin(llmResponseText(resp), 500), responseErr)
			return nil, nil
		},
		BeforeToolCallback: func(_ tool.Context, t tool.Tool, args map[string]any) (map[string]any, error) {
			log.Printf("adk plugin runtime_audit callback=before_tool tool=%q args=%q", t.Name(), previewForPlugin(redactText(mustJSON(args)), 500))
			return nil, nil
		},
		AfterToolCallback: func(_ tool.Context, t tool.Tool, args, result map[string]any, err error) (map[string]any, error) {
			status := "success"
			if err != nil {
				status = "error"
			}
			log.Printf("adk plugin runtime_audit callback=after_tool tool=%q status=%s result=%q error=%v", t.Name(), status, previewForPlugin(redactText(mustJSON(result)), 500), err)
			return nil, nil
		},
		OnToolErrorCallback: func(_ tool.Context, t tool.Tool, args map[string]any, err error) (map[string]any, error) {
			log.Printf("adk plugin runtime_audit callback=on_tool_error tool=%q args=%q error=%v", t.Name(), previewForPlugin(redactText(mustJSON(args)), 500), err)
			return nil, nil
		},
		OnEventCallback: func(_ agent.InvocationContext, event *session.Event) (*session.Event, error) {
			if event != nil {
				log.Printf("adk plugin runtime_audit callback=on_event author=%q final=%t output=%q", event.Author, event.IsFinalResponse(), previewForPlugin(llmResponseText(&event.LLMResponse), 500))
			}
			return event, nil
		},
	})
}

func newSensitiveDataMaskPlugin() (*plugin.Plugin, error) {
	return plugin.New(plugin.Config{
		Name: "sensitive_data_mask",
		OnUserMessageCallback: func(_ agent.InvocationContext, content *genai.Content) (*genai.Content, error) {
			masked := redactContent(content)
			if masked == content {
				return nil, nil
			}
			return masked, nil
		},
		BeforeModelCallback: func(_ agent.CallbackContext, req *model.LLMRequest) (*model.LLMResponse, error) {
			if req == nil {
				return nil, nil
			}
			for i, content := range req.Contents {
				req.Contents[i] = redactContent(content)
			}
			if req.Config != nil && req.Config.SystemInstruction != nil {
				req.Config.SystemInstruction = redactContent(req.Config.SystemInstruction)
			}
			return nil, nil
		},
		AfterModelCallback: func(_ agent.CallbackContext, resp *model.LLMResponse, responseErr error) (*model.LLMResponse, error) {
			if resp == nil || responseErr != nil {
				return nil, nil
			}
			maskedContent := redactContent(resp.Content)
			if maskedContent == resp.Content {
				return nil, nil
			}
			masked := *resp
			masked.Content = maskedContent
			return &masked, nil
		},
	})
}

func newConfirmationGuardPlugin() (*plugin.Plugin, error) {
	return plugin.New(plugin.Config{
		Name: "confirmation_guard",
		BeforeToolCallback: func(_ tool.Context, t tool.Tool, args map[string]any) (map[string]any, error) {
			reason := highRiskToolReason(t.Name(), args)
			if reason == "" {
				return nil, nil
			}
			return map[string]any{
				"status":  "blocked",
				"action":  "requires_confirmation",
				"message": "High-risk tool call blocked before execution: " + reason,
				"tool":    t.Name(),
			}, nil
		},
	})
}

func newPermissionGuardPlugin() (*plugin.Plugin, error) {
	return newPermissionGuardPluginWithConfig(nil)
}

func newPermissionGuardPluginWithConfig(config map[string]any) (*plugin.Plugin, error) {
	denyTools := configSet(config, "deny_tools", "ADK_PERMISSION_DENY_TOOLS")
	allowAgents := configSet(config, "agent_allowlist", "ADK_PERMISSION_ALLOW_AGENTS")
	blockHighRisk := configBool(config, "block_high_risk", "ADK_PERMISSION_BLOCK_HIGH_RISK", true)
	return plugin.New(plugin.Config{
		Name: "permission_guard",
		BeforeToolCallback: func(ctx tool.Context, t tool.Tool, args map[string]any) (map[string]any, error) {
			agentName := ""
			if ctx != nil {
				agentName = strings.ToLower(strings.TrimSpace(ctx.AgentName()))
			}
			if len(allowAgents) > 0 && !allowAgents[agentName] {
				return policyBlockedToolResult("agent is not allowed to call tools", t.Name()), nil
			}
			toolName := strings.ToLower(strings.TrimSpace(t.Name()))
			if denyTools[toolName] {
				return policyBlockedToolResult("tool is denied by permission policy", t.Name()), nil
			}
			if blockHighRisk && highRiskToolReason(t.Name(), args) != "" {
				return policyBlockedToolResult("high-risk tool is denied by permission policy", t.Name()), nil
			}
			return nil, nil
		},
	})
}

func newOutputPolicyPlugin() (*plugin.Plugin, error) {
	return newOutputPolicyPluginWithConfig(nil)
}

func newOutputPolicyPluginWithConfig(config map[string]any) (*plugin.Plugin, error) {
	blockedPatterns := configStringSlice(config, "blocked_patterns")
	return plugin.New(plugin.Config{
		Name: "output_policy",
		AfterModelCallback: func(_ agent.CallbackContext, resp *model.LLMResponse, responseErr error) (*model.LLMResponse, error) {
			if resp == nil || responseErr != nil {
				return nil, nil
			}
			text := llmResponseText(resp)
			if reason := outputPolicyViolation(text, blockedPatterns); reason != "" {
				message := "Output blocked by policy: " + reason
				blocked := *resp
				blocked.Content = genai.NewContentFromText(message, genai.RoleModel)
				blocked.ErrorCode = "OUTPUT_POLICY_BLOCKED"
				blocked.ErrorMessage = message
				return &blocked, nil
			}
			return nil, nil
		},
	})
}

type skillUsageRecord struct {
	InvokeCount int
	Success     int
	Failure     int
	DurationMS  int
	LastTool    string
	LastStatus  string
	LastAt      time.Time
}

var skillUsageStats = struct {
	sync.Mutex
	started map[string]time.Time
	records map[string]*skillUsageRecord
}{
	started: map[string]time.Time{},
	records: map[string]*skillUsageRecord{},
}

func newSkillUsageTrackerPlugin() (*plugin.Plugin, error) {
	return plugin.New(plugin.Config{
		Name: "skill_usage_tracker",
		BeforeToolCallback: func(_ tool.Context, t tool.Tool, args map[string]any) (map[string]any, error) {
			if !isSkillTool(t.Name(), args) {
				return nil, nil
			}
			key := toolInvocationKey(t.Name(), args)
			skillUsageStats.Lock()
			skillUsageStats.started[key] = time.Now()
			skillUsageStats.Unlock()
			return nil, nil
		},
		AfterToolCallback: func(_ tool.Context, t tool.Tool, args, result map[string]any, err error) (map[string]any, error) {
			if !isSkillTool(t.Name(), args) {
				return nil, nil
			}
			recordSkillUsage(t.Name(), args, err == nil)
			return nil, nil
		},
		OnToolErrorCallback: func(_ tool.Context, t tool.Tool, args map[string]any, err error) (map[string]any, error) {
			if !isSkillTool(t.Name(), args) {
				return nil, nil
			}
			recordSkillUsage(t.Name(), args, false)
			return nil, nil
		},
	})
}

func newCostGuardPlugin() (*plugin.Plugin, error) {
	return newCostGuardPluginWithConfig(nil)
}

func newCostGuardPluginWithConfig(config map[string]any) (*plugin.Plugin, error) {
	cfg := costGuardConfigFromConfig(config)
	return plugin.New(plugin.Config{
		Name: "cost_guard",
		BeforeModelCallback: func(_ agent.CallbackContext, req *model.LLMRequest) (*model.LLMResponse, error) {
			if req == nil {
				return nil, nil
			}
			if req.Model == "" {
				req.Model = cfg.DefaultModel
			}
			promptTokens := estimateTextTokens(llmRequestPreview(req))
			if cfg.MaxPromptTokens > 0 && promptTokens > cfg.MaxPromptTokens {
				if cfg.FallbackModel != "" {
					log.Printf("adk plugin cost_guard action=fallback reason=prompt_tokens model=%q fallback=%q prompt_tokens=%d max=%d", req.Model, cfg.FallbackModel, promptTokens, cfg.MaxPromptTokens)
					req.Model = cfg.FallbackModel
					return nil, nil
				}
				return blockedModelResponse("Prompt token budget exceeded."), nil
			}
			if cfg.BlockedModels[strings.ToLower(req.Model)] || (cfg.BlockPremiumModels && isPremiumModel(req.Model)) {
				if cfg.FallbackModel != "" {
					log.Printf("adk plugin cost_guard action=fallback reason=blocked_model model=%q fallback=%q", req.Model, cfg.FallbackModel)
					req.Model = cfg.FallbackModel
					return nil, nil
				}
				return blockedModelResponse("Model is blocked by cost policy."), nil
			}
			return nil, nil
		},
	})
}

func newModelRouterPlugin() (*plugin.Plugin, error) {
	return newModelRouterPluginWithConfig(nil)
}

func newModelRouterPluginWithConfig(config map[string]any) (*plugin.Plugin, error) {
	cfg := modelRouterConfigFromConfig(config)
	return plugin.New(plugin.Config{
		Name: "model_router",
		BeforeModelCallback: func(ctx agent.CallbackContext, req *model.LLMRequest) (*model.LLMResponse, error) {
			if req == nil {
				return nil, nil
			}
			current := strings.TrimSpace(req.Model)
			if agentModel := cfg.AgentModels[callbackAgentName(ctx)]; agentModel != "" {
				req.Model = agentModel
				log.Printf("adk plugin model_router action=route reason=agent model=%q previous=%q", req.Model, current)
				return nil, nil
			}
			prompt := llmRequestPreview(req)
			promptTokens := estimateTextTokens(prompt)
			if cfg.LongContextModel != "" && cfg.LongContextThreshold > 0 && promptTokens >= cfg.LongContextThreshold {
				req.Model = cfg.LongContextModel
				log.Printf("adk plugin model_router action=route reason=long_context model=%q previous=%q prompt_tokens=%d threshold=%d", req.Model, current, promptTokens, cfg.LongContextThreshold)
				return nil, nil
			}
			if cfg.CodeModel != "" && looksLikeCodeTask(prompt) {
				req.Model = cfg.CodeModel
				log.Printf("adk plugin model_router action=route reason=code_task model=%q previous=%q", req.Model, current)
				return nil, nil
			}
			if cfg.DefaultModel != "" && strings.TrimSpace(req.Model) == "" {
				req.Model = cfg.DefaultModel
			}
			return nil, nil
		},
	})
}

func recordSkillUsage(toolName string, args map[string]any, success bool) {
	key := toolInvocationKey(toolName, args)
	started := time.Now()
	skillUsageStats.Lock()
	if value, ok := skillUsageStats.started[key]; ok {
		started = value
		delete(skillUsageStats.started, key)
	}
	record := skillUsageStats.records[toolName]
	if record == nil {
		record = &skillUsageRecord{LastTool: toolName}
		skillUsageStats.records[toolName] = record
	}
	record.InvokeCount++
	if success {
		record.Success++
		record.LastStatus = "success"
	} else {
		record.Failure++
		record.LastStatus = "failure"
	}
	record.DurationMS += int(time.Since(started).Milliseconds())
	record.LastAt = time.Now()
	snapshot := *record
	skillUsageStats.Unlock()
	log.Printf("adk plugin skill_usage_tracker tool=%q status=%s invoke_count=%d success=%d failure=%d duration_ms=%d", toolName, snapshot.LastStatus, snapshot.InvokeCount, snapshot.Success, snapshot.Failure, int(time.Since(started).Milliseconds()))
}

type costGuardConfig struct {
	MaxPromptTokens    int
	BlockedModels      map[string]bool
	FallbackModel      string
	DefaultModel       string
	BlockPremiumModels bool
}

func costGuardConfigFromEnv() costGuardConfig {
	return costGuardConfigFromConfig(nil)
}

func costGuardConfigFromConfig(config map[string]any) costGuardConfig {
	return costGuardConfig{
		MaxPromptTokens:    configInt(config, "max_prompt_tokens", "ADK_COST_MAX_PROMPT_TOKENS", 0),
		BlockedModels:      configSet(config, "blocked_models", "ADK_COST_BLOCKED_MODELS"),
		FallbackModel:      configString(config, "fallback_model", "ADK_COST_FALLBACK_MODEL"),
		DefaultModel:       configString(config, "default_model", "ADK_COST_DEFAULT_MODEL"),
		BlockPremiumModels: configBool(config, "block_premium_models", "ADK_COST_BLOCK_PREMIUM_MODELS", true),
	}
}

type modelRouterConfig struct {
	DefaultModel         string
	CodeModel            string
	LongContextModel     string
	LongContextThreshold int
	AgentModels          map[string]string
}

func modelRouterConfigFromEnv() modelRouterConfig {
	return modelRouterConfigFromConfig(nil)
}

func modelRouterConfigFromConfig(config map[string]any) modelRouterConfig {
	return modelRouterConfig{
		DefaultModel:         configString(config, "default_model", "ADK_ROUTER_DEFAULT_MODEL"),
		CodeModel:            configString(config, "code_model", "ADK_ROUTER_CODE_MODEL"),
		LongContextModel:     configString(config, "long_context_model", "ADK_ROUTER_LONG_CONTEXT_MODEL"),
		LongContextThreshold: configInt(config, "long_context_tokens", "ADK_ROUTER_LONG_CONTEXT_TOKENS", 8000),
		AgentModels:          configMap(config, "agent_models", "ADK_ROUTER_AGENT_MODELS"),
	}
}

func blockedModelResponse(message string) *model.LLMResponse {
	return &model.LLMResponse{
		Content:      genai.NewContentFromText(message, genai.RoleModel),
		ErrorCode:    "MODEL_POLICY_BLOCKED",
		ErrorMessage: message,
		TurnComplete: true,
	}
}

func isPremiumModel(name string) bool {
	modelName := strings.ToLower(strings.TrimSpace(name))
	if modelName == "" {
		return false
	}
	patterns := []string{"opus", "gpt-4.5", "gpt-4o", "gpt-5", "o1", "o3", "pro", "ultra"}
	for _, pattern := range patterns {
		if strings.Contains(modelName, pattern) {
			return true
		}
	}
	return false
}

func looksLikeCodeTask(text string) bool {
	lower := strings.ToLower(text)
	keywords := []string{"code", "golang", "typescript", "javascript", "python", "sql", "debug", "stack trace", "function", "class", "编程", "代码", "报错", "实现"}
	for _, keyword := range keywords {
		if strings.Contains(lower, keyword) {
			return true
		}
	}
	return strings.Contains(text, "```")
}

func callbackAgentName(ctx agent.CallbackContext) string {
	if ctx == nil {
		return ""
	}
	return strings.TrimSpace(ctx.AgentName())
}

func estimateTextTokens(text string) int {
	return estimateTokens(text)
}

func redactContent(content *genai.Content) *genai.Content {
	if content == nil {
		return nil
	}
	parts := make([]*genai.Part, 0, len(content.Parts))
	changed := false
	for _, part := range content.Parts {
		if part == nil {
			continue
		}
		next := *part
		if next.Text != "" {
			masked := redactText(next.Text)
			if masked != next.Text {
				changed = true
				next.Text = masked
			}
		}
		parts = append(parts, &next)
	}
	if !changed {
		return content
	}
	return &genai.Content{Role: content.Role, Parts: parts}
}

var redactionPatterns = []struct {
	re          *regexp.Regexp
	replacement string
}{
	{regexp.MustCompile(`(?i)(sk-[a-z0-9][a-z0-9_\-]{12,})`), "[REDACTED_API_KEY]"},
	{regexp.MustCompile(`(?i)(api[_-]?key|access[_-]?token|refresh[_-]?token|token|secret|password)\s*[:=]\s*['"]?[^'"\s,;]+`), "$1=[REDACTED_SECRET]"},
	{regexp.MustCompile(`(?i)(postgres|mysql|mongodb|redis)://[^\s'"<>]+`), "[REDACTED_CONNECTION_STRING]"},
	{regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`), "[REDACTED_EMAIL]"},
	{regexp.MustCompile(`\b(?:\+?86[-\s]?)?1[3-9]\d{9}\b`), "[REDACTED_PHONE]"},
}

func redactText(text string) string {
	masked := text
	for _, pattern := range redactionPatterns {
		masked = pattern.re.ReplaceAllString(masked, pattern.replacement)
	}
	return masked
}

func llmRequestPreview(req *model.LLMRequest) string {
	if req == nil {
		return ""
	}
	parts := make([]string, 0, len(req.Contents))
	for _, content := range req.Contents {
		if text := contentText(content); text != "" {
			parts = append(parts, text)
		}
	}
	return redactText(strings.Join(parts, "\n"))
}

func previewForPlugin(text string, limit int) string {
	text = strings.TrimSpace(redactText(text))
	if limit <= 0 || len([]rune(text)) <= limit {
		return text
	}
	runes := []rune(text)
	return string(runes[:limit]) + "..."
}

func mustJSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return string(data)
}

func highRiskToolReason(toolName string, args map[string]any) string {
	name := strings.ToLower(strings.TrimSpace(toolName))
	argsText := strings.ToLower(mustJSON(args))
	riskyNames := []string{"delete", "remove", "unlink", "write_file", "exec", "shell", "bash", "powershell", "sql_exec", "db_write", "drop", "truncate"}
	for _, item := range riskyNames {
		if strings.Contains(name, item) {
			return "tool name matches high-risk operation " + item
		}
	}
	riskyArgs := []string{"rm -rf", "del /", "remove-item", "drop table", "truncate table", "delete from", "chmod 777", "format "}
	for _, item := range riskyArgs {
		if strings.Contains(argsText, item) {
			return "tool arguments match high-risk pattern " + item
		}
	}
	return ""
}

func policyBlockedToolResult(message string, toolName string) map[string]any {
	return map[string]any{
		"status":  "blocked",
		"action":  "permission_denied",
		"tool":    toolName,
		"message": message,
	}
}

func outputPolicyViolation(text string, blockedPatterns []string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ""
	}
	if redactText(trimmed) != trimmed {
		return "sensitive data leak detected"
	}
	lower := strings.ToLower(trimmed)
	patterns := []string{"rm -rf", "drop table", "truncate table", "delete from", "format c:", "chmod 777", "curl ", " | sh", "powershell -enc"}
	patterns = append(patterns, blockedPatterns...)
	for _, pattern := range patterns {
		if strings.Contains(lower, strings.ToLower(pattern)) {
			return "dangerous command pattern " + pattern
		}
	}
	return ""
}

func isSkillTool(toolName string, args map[string]any) bool {
	name := strings.ToLower(strings.TrimSpace(toolName))
	if strings.Contains(name, "skill") {
		return true
	}
	for key, value := range args {
		key = strings.ToLower(key)
		if strings.Contains(key, "skill") {
			return true
		}
		if text, ok := value.(string); ok && strings.Contains(strings.ToLower(text), "skill") {
			return true
		}
	}
	return false
}

func toolInvocationKey(toolName string, args map[string]any) string {
	hash := sha256.Sum256([]byte(toolName + ":" + mustJSON(args)))
	return hex.EncodeToString(hash[:])
}

func envInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envBool(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return envBoolValue(value, fallback)
}

func envSet(key string) map[string]bool {
	result := map[string]bool{}
	for _, item := range strings.Split(os.Getenv(key), ",") {
		item = strings.ToLower(strings.TrimSpace(item))
		if item != "" {
			result[item] = true
		}
	}
	return result
}

func envMap(key string) map[string]string {
	result := map[string]string{}
	for _, item := range strings.Split(os.Getenv(key), ",") {
		parts := strings.SplitN(item, ":", 2)
		if len(parts) != 2 {
			continue
		}
		name := strings.TrimSpace(parts[0])
		modelName := strings.TrimSpace(parts[1])
		if name != "" && modelName != "" {
			result[name] = modelName
		}
	}
	return result
}

func configString(config map[string]any, field string, envKey string) string {
	if value, ok := config[field]; ok {
		if text, ok := value.(string); ok {
			return strings.TrimSpace(text)
		}
	}
	return strings.TrimSpace(os.Getenv(envKey))
}

func configInt(config map[string]any, field string, envKey string, fallback int) int {
	if value, ok := config[field]; ok {
		switch typed := value.(type) {
		case float64:
			return int(typed)
		case int:
			return typed
		case string:
			if parsed, err := strconv.Atoi(strings.TrimSpace(typed)); err == nil {
				return parsed
			}
		}
	}
	return envInt(envKey, fallback)
}

func configBool(config map[string]any, field string, envKey string, fallback bool) bool {
	if value, ok := config[field]; ok {
		switch typed := value.(type) {
		case bool:
			return typed
		case string:
			return envBoolValue(typed, fallback)
		}
	}
	return envBool(envKey, fallback)
}

func configSet(config map[string]any, field string, envKey string) map[string]bool {
	result := map[string]bool{}
	for _, item := range configStringSlice(config, field) {
		item = strings.ToLower(strings.TrimSpace(item))
		if item != "" {
			result[item] = true
		}
	}
	if len(result) > 0 {
		return result
	}
	return envSet(envKey)
}

func configStringSlice(config map[string]any, field string) []string {
	value, ok := config[field]
	if !ok {
		return nil
	}
	switch typed := value.(type) {
	case []any:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
				items = append(items, strings.TrimSpace(text))
			}
		}
		return items
	case []string:
		return typed
	case string:
		return splitCSV(typed)
	default:
		return nil
	}
}

func configMap(config map[string]any, field string, envKey string) map[string]string {
	if value, ok := config[field]; ok {
		result := map[string]string{}
		if raw, ok := value.(map[string]any); ok {
			for key, item := range raw {
				if text, ok := item.(string); ok && strings.TrimSpace(key) != "" && strings.TrimSpace(text) != "" {
					result[strings.TrimSpace(key)] = strings.TrimSpace(text)
				}
			}
		}
		if raw, ok := value.(map[string]string); ok {
			for key, text := range raw {
				if strings.TrimSpace(key) != "" && strings.TrimSpace(text) != "" {
					result[strings.TrimSpace(key)] = strings.TrimSpace(text)
				}
			}
		}
		if len(result) > 0 {
			return result
		}
	}
	return envMap(envKey)
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			items = append(items, part)
		}
	}
	return items
}

func envBoolValue(value string, fallback bool) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}
