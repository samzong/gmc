# GMA (Git Message Assistant) 技术设计方案

## 项目概述

GMA 是一个加速 Git add 和 commit 效率的 CLI 工具，通过 LLM 智能生成高质量的 commit message，减少开发者在提交代码时的心智负担。

## 核心功能

1. **快速提交**：一键完成 git add 和 commit 操作
2. **智能消息生成**：基于 git diff 自动生成符合规范的 commit message
3. **多模型支持**：支持 OpenAI API 规范
4. **角色定制**：根据不同工程师角色生成针对性的 commit message
5. **符合规范**：生成的消息遵循 Conventional Commits 规范

## 安装方法

### 从源码安装

```bash
# 克隆仓库
git clone https://github.com/samzong/gma.git
cd gma

# 构建和安装
make install
```

### 使用Go安装

```bash
go install github.com/samzong/gma@latest
```

## 使用方法

### 首次使用

首次使用GMA需要设置OpenAI API密钥：

```bash
gma config set apikey YOUR_OPENAI_API_KEY
```

可选：设置LLM模型、角色和API基础URL：

```bash
# 设置模型
gma config set model gpt-4

# 设置角色
gma config set role 前端工程师

# 设置API基础URL（用于代理访问OpenAI API）
gma config set apibase https://your-proxy-domain.com/v1
```

### 基本使用

在Git仓库中，修改代码后，直接运行：

```bash
gma
```

这将自动检测文件变更，生成提交消息，然后执行git add和git commit操作。

### 高级选项

```bash
# 跳过 pre-commit 钩子
gma --no-verify

# 仅生成消息，不实际提交
gma --dry-run

# 使用自定义配置文件
gma --config /path/to/config.yaml
```

## 支持的角色和模型

GMA支持自定义任意角色和模型名称，以下是内置的建议选项：

### 建议角色

| 角色 | 特点 |
|------|------|
| 前端工程师 | 关注 UI 组件、样式和用户体验 |
| 后端工程师 | 关注 API、数据库和业务逻辑 |
| DevOps 工程师 | 关注部署、CI/CD 和基础设施 |
| 全栈工程师 | 平衡前后端关注点 |
| Markdown 工程师 | 关注文档和说明文件 |

您可以根据自己的需要自定义角色名称，例如：
```bash
gma config set role "Python专家"
```

### 建议模型

以下是默认支持的模型，但您可以设置任何支持OpenAI API格式的模型：

- gpt-3.5-turbo (默认)
- gpt-4
- gpt-4-turbo

自定义模型示例：
```bash
gma config set model "your-custom-model"
```

## 技术架构

### 核心模块

1. **命令模块 (cmd)**
   - 使用 Cobra 框架构建命令行接口
   - 提供主命令和子命令，如 `gma`、`gma config` 等
   - `gma` 可一键完成 git diff 分析， git add ， git commit 操作

2. **Git 操作模块 (internal/git)**
   - 封装 Git 操作，获取 diff 信息
   - 分析变更文件类型和变更内容

3. **LLM 集成模块 (internal/llm)**
   - 支持 OpenAI API
   - 统一 LLM 调用接口

4. **配置管理模块 (internal/config)**
   - 管理用户配置文件
   - 提供角色模板管理

5. **消息格式化模块 (internal/formatter)**
   - 将 LLM 生成的内容格式化为规范的 commit message

## 贡献指南

欢迎提交 Issues 和 Pull Requests。

## 许可证

MIT