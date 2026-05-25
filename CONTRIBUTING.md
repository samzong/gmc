# Contributing

Thanks for helping improve gmc.

## Development

```bash
make check
```

Use focused pull requests. Keep CLI behavior backward-compatible unless the change
explicitly documents a migration path.

## Releases

Releases are created from `v*` tags with GoReleaser. Before tagging, validate the
release config locally:

```bash
goreleaser check
goreleaser release --snapshot --clean
```

## License

By contributing, you agree that your contribution is licensed under the MIT
License.
