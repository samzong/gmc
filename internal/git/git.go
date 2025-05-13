package git

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// GetDiff 获取当前工作区的所有diff（包括未暂存和已暂存）
func GetDiff() (string, error) {
	// 获取未暂存的变更
	cmd := exec.Command("git", "diff")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("执行git diff失败: %w", err)
	}
	
	unstaged := out.String()
	out.Reset()
	
	// 获取已暂存的变更
	cmd = exec.Command("git", "diff", "--cached")
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("执行git diff --cached失败: %w", err)
	}
	
	staged := out.String()
	
	// 合并变更
	diff := unstaged + staged
	return diff, nil
}

// GetStagedDiff 只获取已暂存的变更
func GetStagedDiff() (string, error) {
	cmd := exec.Command("git", "diff", "--cached")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("执行git diff --cached失败: %w", err)
	}
	
	return out.String(), nil
}

// ParseChangedFiles 解析所有变更的文件列表（包括未暂存和已暂存）
func ParseChangedFiles() ([]string, error) {
	// 获取未暂存的变更文件
	cmd := exec.Command("git", "diff", "--name-only")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("执行git diff --name-only失败: %w", err)
	}
	
	unstaged := strings.Split(strings.TrimSpace(out.String()), "\n")
	out.Reset()
	
	// 获取已暂存的变更文件
	cmd = exec.Command("git", "diff", "--cached", "--name-only")
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("执行git diff --cached --name-only失败: %w", err)
	}
	
	staged := strings.Split(strings.TrimSpace(out.String()), "\n")
	
	// 合并并去重
	fileMap := make(map[string]bool)
	for _, file := range unstaged {
		if file != "" {
			fileMap[file] = true
		}
	}
	
	for _, file := range staged {
		if file != "" {
			fileMap[file] = true
		}
	}
	
	var changedFiles []string
	for file := range fileMap {
		changedFiles = append(changedFiles, file)
	}
	
	return changedFiles, nil
}

// ParseStagedFiles 只解析已暂存的文件列表
func ParseStagedFiles() ([]string, error) {
	cmd := exec.Command("git", "diff", "--cached", "--name-only")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("执行git diff --cached --name-only失败: %w", err)
	}
	
	stagedFiles := strings.Split(strings.TrimSpace(out.String()), "\n")
	
	// 过滤空字符串
	var result []string
	for _, file := range stagedFiles {
		if file != "" {
			result = append(result, file)
		}
	}
	
	return result, nil
}

// AddAll 执行git add .
func AddAll() error {
	cmd := exec.Command("git", "add", ".")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("执行git add .失败: %w", err)
	}
	return nil
}

// Commit 执行git commit
func Commit(message string, args ...string) error {
	commitArgs := append([]string{"commit", "-m", message}, args...)
	cmd := exec.Command("git", commitArgs...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("执行git commit失败: %w", err)
	}
	return nil
} 