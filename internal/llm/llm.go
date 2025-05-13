package llm

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/samzong/gma/internal/config"
	"github.com/sashabaranov/go-openai"
)

// GenerateCommitMessage 通过LLM生成提交消息
func GenerateCommitMessage(prompt string, model string) (string, error) {
	cfg := config.GetConfig()
	
	if cfg.APIKey == "" {
		return "", errors.New("未设置API密钥，请先设置API密钥：gma config set apikey YOUR_API_KEY")
	}

	// 创建OpenAI客户端配置
	clientConfig := openai.DefaultConfig(cfg.APIKey)
	
	// 如果设置了API基础URL，则使用自定义URL
	if cfg.APIBase != "" {
		clientConfig.BaseURL = cfg.APIBase
	}
	
	client := openai.NewClientWithConfig(clientConfig)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 如果没有指定模型，使用配置中的模型
	if model == "" {
		model = cfg.Model
	}

	messages := []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleSystem,
			Content: "你是一个专业的Git提交消息生成助手，帮助开发者生成符合Conventional Commits规范的提交消息。",
		},
		{
			Role:    openai.ChatMessageRoleUser,
			Content: prompt,
		},
	}

	resp, err := client.CreateChatCompletion(
		ctx,
		openai.ChatCompletionRequest{
			Model:    model,
			Messages: messages,
		},
	)

	if err != nil {
		return "", fmt.Errorf("调用LLM失败: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", errors.New("LLM返回空响应")
	}

	// 处理返回的消息
	return strings.TrimSpace(resp.Choices[0].Message.Content), nil
} 