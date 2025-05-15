# default

```plaintext
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
- Use English for the commit message
- Character length less than 80
```
