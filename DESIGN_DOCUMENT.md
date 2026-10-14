# Design Document: superfile-enhanced

## 1. Original Architecture

### 1.1 superfile (yorukot/superfile)

The original superfile is a modern terminal file manager written in Go using:
- **Bubble Tea** — TUI framework (Elm-like architecture)
- **Lipgloss** — Styling and layout
- **urfave/cli v3** — CLI argument parsing
- **XDG** — Cross-platform config/data directories

Key components:
- **Model** (`src/internal/common/model.go`) — Central state with panels, sidebar, modals
- **File Operations** (`src/internal/common/handle_file_operations.go`) — Copy, move, delete, compress, extract
- **UI Components** — Sidebar, file panel, process bar, help menu, zoxide integration
- **Configuration** — TOML-based config with hotkeys
- **File Preview** — Basic image and text preview via `src/pkg/file_preview/`

### 1.2 Strengths
- Clean TUI architecture with Bubble Tea
- Good test coverage
- Active development and community
- Cross-platform (Linux, macOS, Windows)
- Extensible configuration

### 1.3 Limitations Identified
1. **Local-only** — No cloud storage support
2. **No AI/ML** — Manual file organization only
3. **No plugin system** — Hard to extend without forking
4. **Basic previews** — No syntax highlighting, limited image support
5. **Basic search** — Only filename filtering, no content search
6. **No African fintech context** — Missing mobile money receipt handling

---

## 2. Problems Identified

| Area | Problem | Impact |
|------|---------|--------|
| Cloud | No S3/Azure/GCS/WebDAV support | Cannot manage cloud files in TUI |
| AI | No automatic categorization | Manual file organization |
| Extensibility | No plugin architecture | Fork required for new features |
| Previews | No syntax highlighting | Poor code/file inspection |
| Search | Filename only | Cannot find files by content |
| Domain | No fintech awareness | Missing African mobile money patterns |

---

## 3. Proposed Architecture

### 3.1 High-Level Design

```
┌────────────────────────────────────────────────────────────────┐
│                     superfile-enhanced                          │
├────────────────────────────────────────────────────────────────┤
│  Original superfile (TUI, config, file ops) ← UNCHANGED        │
├────────────────────────────────────────────────────────────────┤
│  New Core Services (loosely coupled, interface-based)          │
│  ┌──────────────┐ ┌──────────────┐ ┌──────────────┐           │
│  │ CloudStorage │ │ Categorize   │ │ Preview      │           │
│  │ Registry     │ │ Engine       │ │ Generator    │           │
│  └──────┬───────┘ └──────┬───────┘ └──────┬───────┘           │
│         │                │                │                     │
│  ┌──────▼────────────────▼────────────────▼──────────────────┐  │
│  │                    Plugin System                            │  │
│  │  ┌─────────┐ ┌─────────┐ ┌─────────┐ ┌─────────┐          │  │
│  │  │Storage  │ │Categoriz│ │ Preview │ │ Search  │  ...     │  │
│  │  │Plugins  │ │er Plugins│ │Plugins  │ │Plugins  │          │  │
│  │  └─────────┘ └─────────┘ └─────────┘ └─────────┘          │  │
│  └────────────────────────────────────────────────────────────┘  │
└────────────────────────────────────────────────────────────────┘
```

### 3.2 Core Services

#### CloudStorage Registry
- **Interface**: `CloudStorageProvider` with 14 methods (List, Stat, Get, Put, Delete, Mkdir, Copy, Move, PresignURL, Capabilities)
- **Providers**: S3, Azure Blob, GCS, WebDAV (4 built-in)
- **Configuration**: TOML-based with multiple named providers
- **Integration**: Sidebar mounts, cloud:// URI scheme

#### Categorize Engine
- **Rule-based** with extensible design for future ML
- **12 built-in categories** + African mobile money (4 providers)
- **Confidence scoring** (0.0-1.0)
- **Batch processing** with statistics

#### Preview Generator
- **Syntax highlighting** via chroma (50+ languages)
- **Image thumbnails** via imaging (kitty/iTerm2 protocol)
- **Markdown rendering** via goldmark
- **PDF placeholder** (pdfium integration planned)

#### Search Engine
- **Ripgrep integration** for content search
- **Fuzzy filename matching** (character sequence)
- **Search indexing** for <1MB files
- **Context lines** for results

#### Plugin System
- **Native Go plugins** (.so via `plugin` package)
- **WASM support** (planned)
- **Manifest-based** with permissions
- **Types**: Storage, Categorizer, Preview, Search, UI

---

## 4. New Architecture Details

### 4.1 Cloud Storage Provider Interface

```go
type CloudStorageProvider interface {
    Name() string
    DisplayName() string
    Init(ctx context.Context, config map[string]string) error
    ValidateConfig(config map[string]string) error
    List(ctx context.Context, path string, opts ListOptions) ([]FileInfo, error)
    Stat(ctx context.Context, path string) (*FileInfo, error)
    Get(ctx context.Context, path string, w io.Writer) error
    Put(ctx context.Context, path string, r io.Reader, opts PutOptions) error
    Delete(ctx context.Context, path string) error
    Mkdir(ctx context.Context, path string) error
    Copy(ctx context.Context, src, dst string) error
    Move(ctx context.Context, src, dst string) error
    PresignURL(ctx context.Context, path string, expiry time.Duration) (string, error)
    Capabilities() ProviderCapabilities
}
```

### 4.2 Provider Implementations

| Provider | SDK | Auth | Features |
|----------|-----|------|----------|
| S3 | AWS SDK v2 | Access Key / IAM / STS | Multipart, Presign, Versioning, Encryption |
| Azure | Azure SDK | Account Key / Managed ID | SAS URLs, Tiering, Versioning |
| GCS | Google Cloud SDK | Service Account / ADC | Signed URLs, Object Versioning |
| WebDAV | gowebdav | Basic Auth | Nextcloud, OwnCloud compatible |

### 4.3 Categorization Rules

```go
type CategorizationRule struct {
    ID          string
    Category    FileCategory
    Patterns    []string      // Glob patterns
    Regexes     []*regexp.Regexp
    MIMETypes   []string
    Extensions  []string
    Keywords    []string      // Content-based
    CustomFunc  func(string) (bool, float64)
    Priority    int           // Higher = matched first
}
```

### 4.4 Plugin Manifest

```json
{
  "name": "my-plugin",
  "version": "1.0.0",
  "type": "storage|categorizer|preview|search|ui",
  "entry_point": "Plugin",
  "permissions": ["fs:read", "net:outbound"],
  "config": { "key": "value" }
}
```

---

## 5. Technology Decisions

| Decision | Rationale |
|----------|-----------|
| **Interface-based design** | Loose coupling, testability, plugin support |
| **AWS SDK v2** | Modern, performant, maintained |
| **Azure SDK** | Official, supports managed identity |
| **Google Cloud SDK** | Official, ADC support |
| **gowebdav** | Mature WebDAV client |
| **chroma** | Best Go syntax highlighter |
| **goldmark** | Fast, extensible markdown |
| **imaging** | Pure Go image processing |
| **Bubble Tea plugins** | Native Go plugin package |

---

## 6. Security Decisions

| Concern | Mitigation |
|---------|------------|
| Credential storage | OS keyring (planned), env vars for now |
| Path traversal | Strict path validation, prefix-based |
| Network exposure | Configurable timeouts, no default public access |
| Audit | Structured logging with correlation IDs |
| Secrets in config | Never — use env vars or keyring |

---

## 7. Performance Decisions

| Area | Approach |
|------|----------|
| Cloud transfers | Multipart upload, streaming download |
| Categorization | Rule priority sorting, early exit at 0.95 confidence |
| Search | Ripgrep for content, fuzzy matching for names |
| Previews | Lazy loading, 1MB text limit, image caching |
| Memory | Bounded caches, streaming for large files |

---

## 8. Scalability Considerations

- **Cloud providers**: Connection pooling via SDK defaults
- **Categorization**: Stateless, horizontally scalable
- **Search**: Index sharding by directory (planned)
- **Plugins**: Isolated processes via WASM (planned)
- **Large directories**: Virtual scrolling in TUI (upstream)

---

## 9. Deployment Architecture

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│   Local     │     │   Cloud     │     │   Plugins   │
│   Files     │     │   Storage   │     │  (~/.config)│
└──────┬──────┘     └──────┬──────┘     └──────┬──────┘
       │                   │                   │
       └───────────────────┼───────────────────┘
                           ▼
              ┌────────────────────────┐
              │    superfile-enhanced  │
              │   (Single Binary)      │
              └────────────────────────┘
```

- Single binary distribution
- Plugins loaded from `~/.config/superfile/plugins/`
- Config in `~/.config/superfile/`
- No external dependencies at runtime (except cloud SDKs)

---

## 10. Testing Strategy

| Level | Tools | Coverage Target |
|-------|-------|-----------------|
| Unit | `testing`, `testify` | >80% |
| Integration | MinIO (S3), Azurite (Azure), GCS Emulator | Key paths |
| E2E | Bubble Tea test helpers | Critical flows |
| Benchmark | `go test -bench` | Regression detection |

---

## 11. Future Improvements

1. **ML Categorization** — TensorFlow Lite models for content-based classification
2. **WASM Plugins** — Sandboxed, cross-platform plugins
3. **Distributed Index** — SQLite-based search index with background updates
4. **Sync Engine** — Bidirectional sync between cloud providers
5. **Web UI** — Optional web interface for remote access
6. **African Cloud Templates** — Pre-configured providers for Africa Data Centres, Teraco, etc.