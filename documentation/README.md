# AcuGIS Report Server documentation

MkDocs Material site for AcuGIS Report Server.

Documentation is organized by audience:

| Section | Audience |
|---------|----------|
| **Getting Started** | First-time setup and concepts |
| **User Guide** | Report users — product features without REST/CLI/SDK noise |
| **Administration** | Operators — install, secure, deploy, troubleshoot |
| **Developer Guide** | Integrators — REST, CLI, SDKs, OpenAPI, tutorials |
| **Reference** | Lookup tables (endpoints, CLI, env vars, formats) |
| **Release Notes** | Change tracking |

## Setup

```bash
python -m pip install -r requirements.txt
```

## Local preview

```bash
cd documentation
python -m mkdocs serve
```

Open [http://127.0.0.1:8000](http://127.0.0.1:8000).

## Strict build

```bash
cd documentation
python -m mkdocs build --strict
```

Output is written to `site/` (gitignored).

## Versioning

This project includes [mike](https://github.com/jimporter/mike) for multi-version docs. Configure versions when publishing; local development uses `mkdocs serve` / `mkdocs build` as usual.
