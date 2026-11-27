# Contributing

Thanks for helping. Issues and pull requests are welcome.

## Develop

```bash
go build ./...
go vet ./...
go test ./...
CGO_ENABLED=0 go test ./...   # the pure-Go build must pass too
```

Go 1.24 or newer. With cgo enabled, the tree-sitter grammars need a C compiler.

## Guidelines

- Keep the CLI and the skills in step. `cli/cli_test.go` fails if a skill mentions an `qrtx` command that does not exist.
- A change to the manifest format needs a test and an update to `docs/format.md`. `examples/tic-tac-toe` must keep passing `qrtx change verify` (`TestExampleChangeIsValid`).
- New exported API is unstable until it is listed as stable in `doc.go`. Prefer unexported helpers.
- Golden fixtures live in `testdata/fixtures/`. Update them in the same change as the behaviour they pin.

## License

By contributing, you agree that your contributions are licensed under the Apache License 2.0 (see [LICENSE](LICENSE)).
