# Glow

Render markdown on the CLI, with _pizzazz_!

<p align="center">
    <img src="https://stuff.charm.sh/glow/glow-banner-github.gif" alt="Glow Logo">
    <a href="https://github.com/charmbracelet/glow/releases"><img src="https://img.shields.io/github/release/charmbracelet/glow.svg" alt="Latest Release"></a>
    <a href="https://pkg.go.dev/github.com/charmbracelet/glow?tab=doc"><img src="https://godoc.org/github.com/golang/gddo?status.svg" alt="GoDoc"></a>
    <a href="https://github.com/charmbracelet/glow/actions"><img src="https://github.com/charmbracelet/glow/workflows/build/badge.svg" alt="Build Status"></a>
    <a href="https://goreportcard.com/report/github.com/charmbracelet/glow"><img src="https://goreportcard.com/badge/charmbracelet/glow" alt="Go ReportCard"></a>
</p>

<p align="center">
    <img src="https://github.com/user-attachments/assets/c2246366-f84b-4847-b431-32a61ca07b74" width="800" alt="Glow UI Demo">
</p>

## What is it?

Glow is a terminal based markdown reader designed from the ground up to bring
out the beauty—and power—of the CLI.

Use it to discover markdown files, read documentation directly on the command
line. Glow will find local markdown files in subdirectories or a local
Git repository.

## Installation

### Package Manager

```bash
# macOS or Linux
brew install glow
```

```bash
# macOS (with MacPorts)
sudo port install glow
```

```bash
# Arch Linux (btw)
pacman -S glow
```

```bash
# Void Linux
xbps-install -S glow
```

```bash
# Nix shell
nix-shell -p glow --command glow
```

```bash
# FreeBSD
pkg install glow
```

```bash
# Solus
eopkg install glow
```

```bash
# Windows (with Chocolatey, Scoop, or Winget)
choco install glow
scoop install glow
winget install charmbracelet.glow
```

```bash
# Android (with termux)
pkg install glow
```

```bash
# Ubuntu (Snapcraft)
sudo snap install glow
```

```bash
# Debian/Ubuntu
sudo mkdir -p /etc/apt/keyrings
curl -fsSL https://repo.charm.sh/apt/gpg.key | sudo gpg --dearmor -o /etc/apt/keyrings/charm.gpg
echo "deb [signed-by=/etc/apt/keyrings/charm.gpg] https://repo.charm.sh/apt/ * *" | sudo tee /etc/apt/sources.list.d/charm.list
sudo apt update && sudo apt install glow
```

```bash
# Fedora/RHEL
echo '[charm]
name=Charm
baseurl=https://repo.charm.sh/yum/
enabled=1
gpgcheck=1
gpgkey=https://repo.charm.sh/yum/gpg.key' | sudo tee /etc/yum.repos.d/charm.repo
sudo yum install glow
```

Or download a binary from the [releases][releases] page. MacOS, Linux, Windows,
FreeBSD and OpenBSD binaries are available, as well as Debian, RPM, and Alpine
packages. ARM builds are also available for macOS, Linux, FreeBSD and OpenBSD.

### Go

Or just install it with `go`:

```bash
go install charm.land/glow/v3@latest
```

### Build (requires Go 1.21+)

```bash
git clone https://github.com/charmbracelet/glow.git
cd glow
go build
```

[releases]: https://github.com/charmbracelet/glow/releases

## The TUI

Simply run `glow` without arguments to start the textual user interface and
browse local. Glow will find local markdown files in the
current directory and below or, if you’re in a Git repository, Glow will search
the repo.

Markdown files can be read with Glow's high-performance pager. Most of the
keystrokes you know from `less` are the same, but you can press `?` to list
the hotkeys.

## The CLI

In addition to a TUI, Glow has a CLI for working with Markdown. To format a
document use a markdown source as the primary argument:

```bash
# Read from file
glow README.md

# Read from stdin
echo "[Glow](https://github.com/charmbracelet/glow)" | glow -

# Fetch README from GitHub / GitLab
glow github.com/charmbracelet/glow

# Fetch markdown from HTTP
glow https://host.tld/file.md
```

### Word Wrapping

The `-w` flag lets you set a maximum width at which the output will be wrapped:

```bash
glow -w 60
```

### Paging

CLI output can be displayed in your preferred pager with the `-p` flag. This defaults
to the ANSI-aware `less -r` if `$PAGER` is not explicitly set.

### Styles

You can choose a style with the `-s` flag. When no flag is provided `glow` tries
to detect your terminal's current background color and automatically picks
either the `dark` or the `light` style for you.

```bash
glow -s [dark|light]
```

Alternatively you can also supply a custom JSON stylesheet:

```bash
glow -s mystyle.json
```

For additional usage details see:

```bash
glow --help
```

Check out the [Glamour Style Section](https://github.com/charmbracelet/glamour/blob/master/styles/gallery/README.md)
to find more styles. Or [make your own](https://github.com/charmbracelet/glamour/tree/master/styles)!

## Mermaid diagrams

This fork renders ```` ```mermaid ```` code blocks as unicode box-drawing
art (`mmd/` package), laid out and drawn in pure Go:

- Flowcharts (`graph`/`flowchart`, TD/TB/LR/RL directions): nodes in all
  the common shapes (`[rect]`, `(round)`, `[[double]]`, `((circle))`,
  `([stadium])`, `[(cylinder)]`, `{rhombus}`, `{{hexagon}}`), edge styles
  `-->`, `---`, `-.->`, `==>` with pipe (`-->|label|`) and bare
  (`-- label -->`) labels, chained edges, self-loops, multi-line labels
  (`<br/>`), and `subgraph` group frames. Cycles keep entry nodes on top
  by drawing feedback edges back toward them; LR/RL feedback edges take a
  lane outside the diagram instead of crossing boxes.
- Sequence diagrams (`sequenceDiagram`): participants and actors (with
  `as` aliases), all message arrows (`->`, `->>`, `-->`, `-->>`, `-x`,
  `-)` ...), self messages, `Note left of`/`right of`/`over`, and
  `alt`/`else`, `loop`, `opt`, `par`/`and` frames.

Unsupported diagram kinds, directions (`graph BT`), or constructs
(`A --> B & C`, semicolons, edges targeting a subgraph itself, gantt and
other diagram types) make the whole block render as-is — you get the
mermaid source, never a half-drawn diagram. The same applies when parsing
fails. Diagrams inside blockquotes render with their `>` prefix
preserved; indented-code "blocks" stay untouched, as they are not
fences. Subgraph titles clip to their frame's width (the frame never
exceeds the boxes it wraps). Wide diagrams wrap when your terminal (or
`--width`) is too narrow — give them room, like any code block.

## The Config File

If you find yourself supplying the same flags to `glow` all the time, it's
probably a good idea to create a config file. Run `glow config`, which will open
it in your favorite $EDITOR. Alternatively you can manually put a file named
`glow.yml` in the default config path of you platform. If you're not sure where
that is, please refer to `glow --help`.

Here's an example config:

```yaml
# style name or JSON path (default "auto")
style: "light"
# mouse wheel support (TUI-mode only)
mouse: true
# use pager to display markdown
pager: true
# at which column should we word wrap?
width: 80
# show all files, including hidden and ignored.
all: false
# show line numbers (TUI-mode only)
showLineNumbers: false
# preserve newlines in the output
preserveNewLines: false
```

## Contributing

See [contributing][contribute].

[contribute]: https://github.com/charmbracelet/glow/contribute

## Feedback

We’d love to hear your thoughts on this project. Feel free to drop us a note!

- [Twitter](https://twitter.com/charmcli)
- [The Fediverse](https://mastodon.social/@charmcli)
- [Discord](https://charm.sh/chat)

## License

[MIT](https://github.com/charmbracelet/glow/raw/master/LICENSE)

---

Part of [Charm](https://charm.sh).

<a href="https://charm.sh/"><img alt="The Charm logo" src="https://stuff.charm.sh/charm-badge.jpg" width="400"></a>

Charm热爱开源 • Charm loves open source
