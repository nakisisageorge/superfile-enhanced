# Changelog

All notable changes to this project are documented here.
Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versioning is SemVer.

## [0.2.0] — 2026-10-14

### Fixed
- **Build restored.** The repository did not compile against its pinned SDK
  versions; this release makes `go build ./...`, `go vet ./...` and
  `go test ./...` green.
- `src/preview`: corrected chroma v2 usage — the formatter is stored as an
  interface value with the resolved style passed at `Format()` time (the
  removed `formatter.WithOptions`/`formatters.WithStyle` calls never existed
  in chroma v2), unused `image`/`image/jpeg` imports removed.
- `src/cloudstorage` (Azure): migrated from the wrong `service.Client` +
  `ClientSecretCredential` combination to `container.Client` with a proper
  credential chain — shared-key (account key) or managed identity. Blob item
  fields are now dereferenced safely (`Name` is `*string`), upload metadata is
  converted to `map[string]*string`, and `Copy` builds source URLs from the
  client URL.
- `src/cloudstorage` (Azure): `PresignURL` now really works — it issues a
  blob-scoped read-only SAS via `blob.Client.GetSASURL` when an account key is
  configured (managed identity cannot sign SAS tokens and returns a clear
  error).
- `src/cloudstorage` (GCS): `List` now surfaces synthetic directories through
  `ObjectAttrs.Prefix` (the removed `it.Prefixes()` call never existed);
  `PresignURL` uses `BucketHandle.SignedURL` with client credentials instead
  of a broken `client.Conf()` access.
- `src/cloudstorage` (S3): `Get` no longer wraps a plain `io.Writer` in a
  `WriteAtBuffer` (the S3 downloader writes ranges out of order); it uses
  `io.WriterAt` directly when available and otherwise buffers then copies,
  so downloads stay byte-correct.
- `src/cloudstorage` (WebDAV): adapted to gowebdav v0.13 (`ReadDir`/`Stat`
  return `os.FileInfo`, so `toFileInfo` joins the entry path itself; MOVE is
  `Rename`); insecure TLS is implemented via a cloned default transport and
  remains strictly opt-in.
- `src/categorize`: rule priority is now the primary signal — a higher-priority
  rule wins even when a lower-priority rule scores higher raw confidence
  (previously a MIME-type hit at 0.95 could outrank an equally matched config
  rule evaluated first). Confidence only breaks ties within the same priority.
- `src/categorize`: `python_scripts.CustomFunc` no longer matches every file
  (the stub returned `true, 0.8` unconditionally); it now performs a real
  shebang check on the first line of the file.

### Added
- `.env.example` with safe placeholders for all four cloud providers.
- `.gitignore`, GitHub Actions CI (build + vet + test), CHANGELOG.

## [0.1.0] — 2026-10-12

### Added
- Initial enhancement package: cloud storage providers (S3, Azure, GCS,
  WebDAV), rule-based AI file categorization with African mobile money
  receipt detection (MPESA, MTN MoMo, Airtel Money, Orange Money), plugin
  architecture, enhanced previews (syntax highlighting, images, markdown),
  and semantic search with ripgrep integration.

### Attribution
- Original project: [yorukot/superfile](https://github.com/yorukot/superfile) (MIT).
- This repository is an independent enhancement fork by
  George Nakisisa / GICT Africa Technologies; see PORTFOLIO_CASE_STUDY.md.