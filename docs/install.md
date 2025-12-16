## Install breyta CLI

### Option A: build from source (recommended)

Requirements:
- Go toolchain (recent)

From this repo root:

```bash
cd breyta-cli

go install ./cmd/breyta
```

Ensure `$(go env GOPATH)/bin` is on your `PATH`.

Verify:

```bash
breyta --help
```

### Option B: build a local binary

From `breyta-cli/`:

```bash
make build
./dist/breyta --help
```
