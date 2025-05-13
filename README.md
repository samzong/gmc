# gmc

## Project Overview

`gmc` is a CLI tool that accelerates the efficiency of Git add and commit by using LLM to generate high-quality commit messages, thereby reducing the cognitive load on developers when submitting code.

## Core Features

1. **Quick Commit**：Complete git add and commit operations with a single command
2. **Smart Message Generation**：Automatically generate commit messages based on git diff
3. **Multiple Models Support**：Support OpenAI API
4. **Role Customization**：Generate commit messages tailored to different engineering roles
5. **Conventional Commits**：The generated message follows the Conventional Commits specification

## Usage

First use gmc to set the OpenAI API key:

```bash
gmc config set apikey YOUR_OPENAI_API_KEY
```

Optional: Set LLM model, role, and API base URL:

```bash
# Set model
gmc config set model gpt-4.1-mini

# Set role
gmc config set role Frontend

# Set API base URL (for proxy access to OpenAI API)
gmc config set apibase https://your-proxy-domain.com/v1

# Skip pre-commit hook
gmc --no-verify

# Generate message only, do not actually commit
gmc --dry-run

# Automatically add all changes to the staging area
gmc --all

# Associate issue number
gmc --issue 123
```

## Prompt template

`gmc` supports custom prompt templates, allowing you to adjust the style of the generated commit message.

#### Built-in Templates

| Template Name | Description                                         |
| -------- | -------------------------------------------- |
| default     | Standard prompt template, generate commit messages that conform to the specification |
| detailed     | Generate more detailed commit messages, including type description and more guidance |
| concise     | Generate concise commit messages                       |
| chinese     | Generate Chinese description commit messages                       |

Set template example:

```bash
# Use built-in template
gmc config set prompt_template detailed
```

#### Custom template

You can create a custom prompt template, the method is as follows:

1. Create a YAML format template file in the `~/.gmc/prompts` directory, for example `my_template.yaml`:

```yaml
name: "My Custom Template"
description: "My team's commit message format"
template: |
  As a {{.Role}}, please generate a commit message that follows the Conventional Commits specification for the following Git changes:

  Changed Files:
  {{.Files}}

  Changed Content:
  {{.Diff}}

  Commit message format requirements:
  - Use the "type(scope): description" format
  - The type must be one of: feat, fix, docs, style, refactor, perf, test, chore
  - The scope should be specific, and the description should be concise
  - Do not include issue numbers
```

2. Use the configuration command to set the custom template:

```bash
# Use custom template (only filename)
gmc config set prompt_template my_template

# Or specify the full path
gmc config set prompt_template /path/to/my_template.yaml
```

3. Custom template directory location:

```bash
# Set custom template directory
gmc config set custom_prompts_dir /path/to/templates
```

#### Template variables

You can use the following variables in the template:

- `{{.Role}}`: The user configured role
- `{{.Files}}`: The list of changed files
- `{{.Diff}}`: Git difference content

## License

MIT
