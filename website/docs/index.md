# Prompt template

`gmc` supports prompt templates, allowing you to adjust the style of the generated commit message.

## Built-in Templates

| Template Name | Description                                         |
| -------- | -------------------------------------------- |
| default     | Standard prompt template, generate commit messages that conform to the specification |

## How to Contribute

- Create the corresponding in docs `{{template_name}}.md`
- Add the corresponding page in `mkdocs.yml > nav:`

```yaml
nav:
  - default: default.md
  - {{template_name}}: {{template_name}}.md
```

