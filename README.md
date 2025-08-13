# gmc - Git Message Commit

<div align="center">
  <img src="./logo.png" alt="gmc logo" width="200" />
  <br />
  <p>gmc is a CLI tool that accelerates the efficiency of Git add and commit by using LLM to generate high-quality commit messages, thereby reducing the cognitive load on developers when submitting code.</p>
  <p>
    <a href="https://github.com/samzong/gmc/releases"><img src="https://img.shields.io/github/v/release/samzong/gmc" alt="Release Version" /></a>
    <a href="https://goreportcard.com/report/github.com/samzong/gmc"><img src="https://goreportcard.com/badge/github.com/samzong/gmc" alt="go report" /></a>
    <a href="https://github.com/samzong/gmc/blob/main/LICENSE"><img src="https://img.shields.io/github/license/samzong/gmc" alt="MIT License" /></a>
  </p>
</div>

## Core Features

1. **One Command Commit**：Complete git add and commit operations with a single command
2. **Smart Message Generation**：Automatically generate commit messages based on git diff
3. **LLM Models Support**：Support OpenAI API Style
4. **Role Customization**：Generate commit messages tailored to different engineering roles
5. **Conventional Commits**：The generated message follows the Conventional Commits specification
6. **Branch Creation**：Automatically create feature branches with generated names based on description
7. **Commit History Analysis**：Analyze commit quality and get AI-powered improvement suggestions

## Usage

First use gmc to set the OpenAI API serivce:

```bash
gmc config set apibase https://your-proxy-domain.com/v1
gmc config set apikey YOUR_OPENAI_API_KEY
gmc config set model gpt-4.1-mini
```

And Configure other parameters.

```bash
# Set role
(base) ➜  ~ gmc config set [Key]
apibase          -- Set OpenAI API Base URL
apikey           -- Set OpenAI API Key
model            -- Set up the LLM model
prompt_template  -- Set Prompt Template
prompts_dir      -- Set Prompt Template Directory
role             -- Set Current Role
```

Use the following command.

```bash
# It's will Automatically read git diff from staging area.
gmc

# Automatically add all changes to the staging area
gmc -a

# Associate issue number
gmc --issue 123

# Create feature branch with generated name
gmc --branch "implement user authentication"

# Verbose output for debugging
gmc --verbose

# Set Template directory
gmc config set prompts_dir /path/to/templates

# Use Template (only filename in prompts_dir)
gmc config set prompt_template my_template

# Analyze commit history quality (personal)
gmc analyze

# Analyze team commit history quality
gmc analyze --team
```

## Prompt template

`gmc` supports prompt templates, allowing you to adjust the style of the generated commit message.

### Built-in Templates

| Template Name | Description                                                                          |
| ------------- | ------------------------------------------------------------------------------------ |
| default       | Standard prompt template, generate commit messages that conform to the specification |

### Template

You can create a prompt template, the method is as follows:

1. Create a YAML format template file in the `~/.gmc/prompts` directory, for example `my_template.yaml`:

```yaml
name: "My Template"
description: "My commit message format"
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

### Template variables

You can use the following variables in the template:

- `{{.Role}}`: The user configured role
- `{{.Files}}`: The list of changed files
- `{{.Diff}}`: Git difference content

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details
