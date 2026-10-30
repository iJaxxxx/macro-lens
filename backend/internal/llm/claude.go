// Package llm 封装对 Claude 的调用。只在配置了 ANTHROPIC_API_KEY 时启用。
package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/anthropics/anthropic-sdk-go/v2/option"
	"github.com/anthropics/anthropic-sdk-go/v2/shared/constant"
)

const defaultModel = "claude-opus-5-5"

// Effort 控制思考深度：简单的分析师角色用 low，最终决策用 medium。
type Effort = anthropic.BetaOutputConfigEffort

const (
	EffortLow    Effort = anthropic.BetaOutputConfigEffortLow
	EffortMedium Effort = anthropic.BetaOutputConfigEffortMedium
)

type Client struct {
	api   anthropic.Client
	model string
}

// FromEnv 在设置了 ANTHROPIC_API_KEY 时返回客户端，否则返回 nil（调用方退回规则引擎）。
// 模型可以用 ANTHROPIC_MODEL 覆盖。
func FromEnv() *Client {
	key := os.Getenv("ANTHROPIC_API_KEY")
	if key == "" {
		return nil
	}
	model := os.Getenv("ANTHROPIC_MODEL")
	if model == "" {
		model = defaultModel
	}
	return New(model, option.WithAPIKey(key))
}

// New 用给定的模型和 SDK 选项创建客户端，测试里可以把 BaseURL 指向假服务器。
func New(model string, opts ...option.RequestOption) *Client {
	return &Client{api: anthropic.NewClient(opts...), model: model}
}

func (c *Client) Model() string { return c.model }

// AskJSON 发一轮对话，要求模型按 schema 输出 JSON，并解码到 out。
func (c *Client) AskJSON(ctx context.Context, system, user string, schema map[string]any, effort Effort, out any) error {
	resp, err := c.api.Beta.Messages.New(ctx, anthropic.BetaMessageNewParams{
		Model:     c.model,
		MaxTokens: 16000,
		System:    []anthropic.BetaTextBlockParam{{Text: system}},
		Messages:  []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(user))},
		OutputConfig: anthropic.BetaOutputConfigParam{
			Effort: effort,
			Format: anthropic.BetaJSONOutputFormatParam{Schema: schema},
		},
		// 安全分类器拒答时由服务端自动换模型重试
		Fallbacks: anthropic.BetaFallbacksParamUnion{OfDefault: constant.ValueOf[constant.Default]()},
		Betas:     []anthropic.AnthropicBeta{anthropic.AnthropicBetaServerSideFallback2026_07_01},
	})
	if err != nil {
		var apiErr *anthropic.Error
		if errors.As(err, &apiErr) {
			return fmt.Errorf("claude: status %d: %w", apiErr.StatusCode, err)
		}
		return fmt.Errorf("claude: %w", err)
	}
	switch resp.StopReason {
	case anthropic.BetaStopReasonRefusal:
		return errors.New("claude: 模型拒绝回答")
	case anthropic.BetaStopReasonMaxTokens:
		return errors.New("claude: 输出被截断")
	}
	var text strings.Builder
	for _, block := range resp.Content {
		if b, ok := block.AsAny().(anthropic.BetaTextBlock); ok {
			text.WriteString(b.Text)
		}
	}
	if err := json.Unmarshal([]byte(text.String()), out); err != nil {
		return fmt.Errorf("claude: 解析 JSON 失败: %w", err)
	}
	return nil
}

// Object 是构造 JSON schema 的小工具：所有字段必填、不允许多余字段（结构化输出要求）。
func Object(props map[string]any) map[string]any {
	required := make([]string, 0, len(props))
	for k := range props {
		required = append(required, k)
	}
	return map[string]any{"type": "object", "properties": props, "required": required, "additionalProperties": false}
}

func String(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }
func Number(desc string) map[string]any { return map[string]any{"type": "number", "description": desc} }
func Enum(desc string, values ...string) map[string]any {
	return map[string]any{"type": "string", "enum": values, "description": desc}
}
func Strings(desc string) map[string]any {
	return map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": desc}
}

