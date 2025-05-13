package formatter

import (
	"fmt"
	"regexp"
	"strings"
)

// BuildPrompt 构建提示词
func BuildPrompt(role string, changedFiles []string, diff string) string {
	// 限制diff的内容大小，避免超出token限制
	if len(diff) > 4000 {
		diff = diff[:4000] + "...(内容过长已截断)"
	}
	
	// 构建变更文件列表字符串
	changedFilesStr := strings.Join(changedFiles, "\n")
	
	// 构建提示词
	prompt := fmt.Sprintf(`你是一个专业的%s，请根据以下Git变更内容，生成一个符合Conventional Commits规范的提交消息：

变更文件：
%s

变更内容：
%s

请生成格式为"类型(范围): 描述"的提交消息。
类型应从以下选择最合适的：feat, fix, docs, style, refactor, perf, test, chore。
描述应简明扼要（不超过150字符），准确反映变更内容。
不要在提交消息中添加issue编号，如"#123"或"(#123)"，这会由工具自动处理。`, role, changedFilesStr, diff)
	
	return prompt
}

// FormatCommitMessage 格式化提交消息
func FormatCommitMessage(message string) string {
	// 移除多余的换行和空格
	message = strings.TrimSpace(message)
	
	// 提取第一行作为提交消息
	lines := strings.Split(message, "\n")
	if len(lines) > 0 {
		firstLine := lines[0]
		
		// 移除可能存在的issue编号引用
		issuePattern := regexp.MustCompile(`\s*\(#\d+\)|\s*#\d+`)
		firstLine = issuePattern.ReplaceAllString(firstLine, "")
		
		// 检查是否符合Conventional Commits规范
		conventionalPattern := regexp.MustCompile(`^(feat|fix|docs|style|refactor|perf|test|chore)(\([^\)]+\))?: .+`)
		if !conventionalPattern.MatchString(firstLine) {
			// 如果不符合规范，尝试格式化
			return formatToConventional(firstLine)
		}
		
		return firstLine
	}
	
	return message
}

// formatToConventional 尝试将消息格式化为Conventional Commits规范
func formatToConventional(message string) string {
	message = strings.TrimSpace(message)
	
	// 尝试推断类型
	var commitType string
	
	lowerMsg := strings.ToLower(message)
	if strings.Contains(lowerMsg, "fix") || strings.Contains(lowerMsg, "bug") || strings.Contains(lowerMsg, "修复") {
		commitType = "fix"
	} else if strings.Contains(lowerMsg, "add") || strings.Contains(lowerMsg, "新增") || strings.Contains(lowerMsg, "feature") {
		commitType = "feat"
	} else if strings.Contains(lowerMsg, "doc") || strings.Contains(lowerMsg, "文档") {
		commitType = "docs"
	} else if strings.Contains(lowerMsg, "style") || strings.Contains(lowerMsg, "样式") {
		commitType = "style"
	} else if strings.Contains(lowerMsg, "refactor") || strings.Contains(lowerMsg, "重构") {
		commitType = "refactor"
	} else if strings.Contains(lowerMsg, "perf") || strings.Contains(lowerMsg, "性能") {
		commitType = "perf"
	} else if strings.Contains(lowerMsg, "test") || strings.Contains(lowerMsg, "测试") {
		commitType = "test"
	} else {
		commitType = "chore"
	}
	
	// 移除可能的前缀
	cleanMessage := message
	prefixes := []string{"fix:", "bug:", "feat:", "feature:", "docs:", "style:", "refactor:", "perf:", "test:", "chore:"}
	for _, prefix := range prefixes {
		if strings.HasPrefix(lowerMsg, prefix) {
			cleanMessage = message[len(prefix):]
			break
		}
	}
	
	cleanMessage = strings.TrimSpace(cleanMessage)
	
	// 组合成符合规范的消息
	return fmt.Sprintf("%s: %s", commitType, cleanMessage)
} 