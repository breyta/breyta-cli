## Install breyta CLI

### Option A: Homebrew (macOS)

Once the Homebrew tap is set up, install/update with:

```bash
brew tap breyta/tap
brew install breyta
```

If you don’t have access to the tap yet, use Option B (download a prebuilt binary).

After install, open a new terminal window and run:

```bash
breyta
```

This opens the TUI. It defaults to prod unless you set `BREYTA_DEV=1` and a local API URL.

### Option B: download a prebuilt binary (no Go required)

Download the latest release artifact for your OS/arch from:

- https://github.com/breyta/breyta-cli/releases

Then extract it and put `breyta` on your `PATH`.

### Option C: build from source (Go required)

This option installs `breyta` into your Go bin directory so you can run it from anywhere.

#### 1) Install Go (required)

`breyta-cli` currently requires **Go 1.23.x** (see `breyta-cli/go.mod`).

- Install Go using the official instructions: [go.dev/doc/install](https://go.dev/doc/install)

Then verify:

```bash
go version
```

If `go version` prints something older than 1.23, upgrade Go.

#### 2) Install `breyta` from this repo

From the **repo root**:

```bash
cd breyta-cli
go install ./cmd/breyta
```

If you want `breyta version` to include the current git commit and install/build time, use:

```bash
make install
```

You can also install a tagged release directly (requires SemVer-shaped tags):

```bash
go install github.com/breyta/breyta-cli/cmd/breyta@v1.2.3
```

#### 3) Make sure Go’s bin directory is on your PATH

The `go install` command places the binary in:

- macOS/Linux: `$(go env GOPATH)/bin`
- Windows: `%USERPROFILE%\go\bin`

Check where Go thinks your `GOPATH` is:

```bash
go env GOPATH
```

If running `breyta --help` fails with “command not found”, add the Go bin path to your `PATH`:

- macOS (zsh default):

```bash
echo 'export PATH="$(go env GOPATH)/bin:$PATH"' >> ~/.zshrc
source ~/.zshrc
```

- Linux (bash):

```bash
echo 'export PATH="$(go env GOPATH)/bin:$PATH"' >> ~/.bashrc
source ~/.bashrc
```

- Windows (PowerShell):

```powershell
setx PATH "$env:USERPROFILE\go\bin;$env:PATH"
```

Then **close and reopen** the terminal.

Verify:

```bash
breyta --help
```

### Option D: build a local binary (Go required)

This builds a binary into `breyta-cli/dist/` and runs it from that folder (useful if you don’t want to touch PATH).

From `breyta-cli/` (macOS/Linux):

```bash
make build
./dist/breyta --help
```

Windows note: `make` is not typically installed by default. Prefer Option A on Windows, or use your team’s Windows dev setup for `make`.

### Troubleshooting

- **`go: command not found`**: Go isn’t installed (or your terminal can’t find it). Install Go and reopen your terminal.
- **`go: go.mod requires go >= 1.23.x`**: Your Go version is too old. Upgrade Go to 1.23.x.
- **`breyta: command not found`**: Your Go bin directory isn’t on `PATH`. Follow the PATH steps above and reopen the terminal.
- **Still stuck**: Share the output of `go version`, `go env GOPATH`, and the exact error you see when running `breyta --help`.
