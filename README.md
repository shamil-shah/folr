# folr

[![CI](https://github.com/shamil-shah/folr/actions/workflows/ci.yml/badge.svg)](https://github.com/shamil-shah/folr/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/Go-1.27%2B-00ADD8?logo=go)](https://go.dev/)

`folr` (short for *folder*) is a fast, security-first, and atomic command-line tool that cleans up cluttered directories by sorting files into categorized folders based on their extensions.

Built for safety and reliability, `folr` guarantees **zero data loss**, **never overwrites files**, **makes zero network requests**, and provides instant **undo** capability.

---

## Quick Demo

```console
$ folr ~/Downloads
Scanning: /home/kali/Downloads
47 files found
Images      (12)  photo.jpg, screen.png, ...
Documents   (9)   report.pdf, cv.docx, ...
Archives    (5)   tools.zip, ...
Videos      (3)   trailer.mp4, ...
Skipped     (4)   incomplete download, hidden file

Proceed? [y/N] y
Moved 43 files.
To revert: folr --undo
```

---

## Key Features

- **Atomic Moves**: Instant `O(1)` renames on the same filesystem; verified SHA-256 copy-and-verify fallback across different filesystems.
- **Full Undo Support**: Every batch is recorded in an encrypted/atomic local JSON session. Run `folr --undo` to seamlessly revert all files to their original locations.
- **Zero Data Loss Guarantee**: `folr` never deletes files. If a file already exists at the destination, it atomically appends a collision suffix (e.g., `photo (1).jpg`).
- **TOCTOU & Race Condition Protection**: Files are re-inspected via `os.Lstat` immediately before moving. If a file was modified or deleted after the plan was generated, it is skipped safely.
- **Directory Locking**: Acquires `.folr.lock` to prevent conflicting concurrent executions.
- **Strict Boundary Defense**: Refuses to run on filesystem roots (`/`, `C:\`), critical system directories (`/etc`, `/usr`, `/boot`, `C:\Windows`, etc.), and user home directories directly.
- **Zero External Telemetry**: 100% offline. Zero network calls, telemetry, or background analytics.

---

## Installation

### Method 1: Pre-built Binaries (Recommended)

Download the ready-to-run binary for your operating system from the **[Releases](https://github.com/shamil-shah/folr/releases)** page.

#### Linux
1. Download the archive for your architecture:
   * 64-bit Intel/AMD: `folr_<version>_linux_amd64.tar.gz`
   * ARM64: `folr_<version>_linux_arm64.tar.gz`
2. Extract the binary and install it globally:
   ```bash
   tar -xzf folr_*_linux_amd64.tar.gz
   chmod +x folr-linux-amd64
   
   # Option A: System-wide for all users (requires sudo)
   sudo mv folr-linux-amd64 /usr/local/bin/folr

   # Option B: For current user only (no sudo required)
   mkdir -p ~/.local/bin
   mv folr-linux-amd64 ~/.local/bin/folr
   ```
3. Test installation:
   ```bash
   folr --version
   ```

#### Windows
1. Download `folr_<version>_windows_amd64.zip` from Releases.
2. Extract `folr.exe` from the zip file.
3. To use `folr` from anywhere in PowerShell or Command Prompt (CMD):
   * **Quickest way**: Move `folr.exe` into `C:\Windows` (already in system PATH, requires admin approval once).
   * **Custom Folder**:
     1. Create a folder, for example `C:\Tools\folr`, and place `folr.exe` inside it.
     2. Press `Win + S`, search for **Environment Variables**, and select **Edit the system environment variables**.
     3. Click **Environment Variables...** button.
     4. Under **User variables** (or System variables), select `Path` and click **Edit**.
     5. Click **New** and add `C:\Tools\folr`.
     6. Click **OK** to save and restart your terminal.
   * **Via PowerShell (One-liner)**:
     ```powershell
     # If placed in C:\Tools\folr, permanently add it to your user PATH:
     [Environment]::SetEnvironmentVariable("Path", $env:Path + ";C:\Tools\folr", "User")
     ```
4. Test in a new PowerShell or CMD window:
   ```powershell
   folr --version
   ```

#### macOS
1. Download `folr_<version>_linux_amd64.tar.gz` (or native binary once available).
2. Extract and move to `/usr/local/bin`:
   ```bash
   tar -xzf folr_*_linux_amd64.tar.gz
   chmod +x folr-linux-amd64
   sudo mv folr-linux-amd64 /usr/local/bin/folr
   ```

---

### Method 2: Install via Go (Cross-Platform)

If you have Go (version 1.23 or newer) installed on your system:
```bash
go install github.com/shamil-shah/folr/cmd/folr@latest
```
*Note: Make sure your `$GOPATH/bin` (or `%USERPROFILE%\go\bin` on Windows) is in your `PATH`.*

---

### Method 3: Build from Source
```bash
# Clone the repository
git clone https://github.com/shamil-shah/folr.git
cd folr

# Build binary for your current OS
go build -o folr ./cmd/folr

# (Optional) Cross-compile for Windows from Linux:
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o folr.exe ./cmd/folr
```

---

## Usage & Commands

```console
folr [flags] [directory]
```

If no directory is specified, `folr` operates on the current working directory.

### Commands & Flags

| Flag | Description |
| ---- | ----------- |
| `folr` | Scans current directory, displays plan, prompts for confirmation, moves files. |
| `folr <path>` | Scans and organizes the specified directory (e.g. `folr ~/Downloads`). |
| `--dry-run` | Preview the organization plan without making any changes to files. |
| `-y`, `--yes` | Skip interactive confirmation and proceed immediately. |
| `--undo` | Reverts the last organization operation in the current directory. |
| `--config <file>` | Path to a custom TOML rules configuration file. |
| `--allow-root` | Explicitly permit execution under root or Administrator privileges. |
| `-v`, `--version` | Display current version of `folr`. |
| `-h`, `--help` | Display usage instructions and help menu. |

> [!NOTE]
> If standard input (`stdin`) is not a terminal (e.g. running from cron or a piped script) and `-y` is not provided, `folr` immediately exits with exit code `2` to prevent unintended changes.

### Exit Codes
- `0`: Success
- `1`: General runtime error
- `2`: Misuse or invalid argument / non-interactive without `-y`
- `3`: Operation rejected due to security constraint (e.g., forbidden directory)

---

## Default Categories

Files are organized based on lowercase extensions:

| Category | Supported Extensions |
| -------- | -------------------- |
| **Images** | `jpg`, `jpeg`, `png`, `gif`, `webp`, `bmp`, `svg`, `heic`, `tiff` |
| **Documents** | `pdf`, `doc`, `docx`, `odt`, `txt`, `rtf`, `md`, `xls`, `xlsx`, `ods`, `ppt`, `pptx`, `csv` |
| **Archives** | `zip`, `rar`, `7z`, `tar`, `gz`, `bz2`, `xz`, `tgz` |
| **Videos** | `mp4`, `mkv`, `avi`, `mov`, `webm`, `flv` |
| **Audio** | `mp3`, `wav`, `flac`, `ogg`, `m4a`, `aac` |
| **Installers** | `exe`, `msi`, `deb`, `rpm`, `dmg`, `appimage`, `apk` |
| **Code** | `py`, `go`, `js`, `ts`, `c`, `cpp`, `java`, `rs`, `sh`, `json`, `yaml`, `yml`, `html`, `css` |
| **Other** | All other file types not matched above |

### Automatically Skipped Files
The following files are left completely untouched:
- **Incomplete downloads**: `.crdownload`, `.part`, `.partial`, `.tmp`, `.download`, `.opdownload`.
- **Hidden files**: Files starting with `.` or with Windows `Hidden` attribute.
- **System files**: Files with Windows `System` attribute.
- **Symbolic links & special devices**: Symlinks, sockets, character devices, FIFOs.
- **Subdirectories**: `folr` only scans direct files; existing subfolders are never traversed or altered.
- **Internal files**: `.folr.toml`, `.folr.lock`.
- **Recently modified files**: Files modified within the last 10 seconds (in case downloads or saves are in progress).

---

## Configuration (`rules.toml`)

You can customize categories and file mappings using a TOML configuration file.

### Lookup Order
1. `--config <path>` flag.
2. Local `.folr.toml` in the target directory.
3. User configuration directory (`~/.config/folr/rules.toml` on Linux, `%APPDATA%\folr\rules.toml` on Windows).
4. Embedded default rules.

### Example `rules.toml`
```toml
[categories]
Images = ["jpg", "png", "webp", "avif"]
Books = ["epub", "mobi", "azw3", "pdf"]
Design = ["psd", "ai", "fig", "sketch"]

[settings]
skip_recent_seconds = 15
```

---

## Safety Guarantees

1. **No Overwrites**: Uses collision resolution (`name (1).ext`) to ensure existing destination files are never clobbered.
2. **Atomic Undo**: During undo, if the source path is now occupied by another file, the restored file is saved as `name (restored).ext`. Empty category directories created during the organize step are cleanly removed when emptied by undo.
3. **Path Traversal Defense**: All target, category, and restored paths are checked against the root working directory using strict relative path bounds checks.
4. **Symlink Defense**: Refuses to move into any category path that is a symbolic link.
5. **Windows Safety**: Prohibits reserved Windows device names (`CON`, `PRN`, `AUX`, `NUL`, `COM1-9`, `LPT1-9`) and illegal characters (`<>:"/\|?*`). Handles paths with spaces seamlessly.

---

## Limitations & Roadmap

- **v0.1 (Current)**: Direct file organization, category rules, atomic undo, and multi-layered security protections.
- **v0.2 (Planned)**: Content-based deduplication (`--dedupe`) using SHA-256 fingerprinting to isolate duplicate files into a `Duplicates` folder without deleting originals.

---

## License

Distributed under the [MIT License](LICENSE).

