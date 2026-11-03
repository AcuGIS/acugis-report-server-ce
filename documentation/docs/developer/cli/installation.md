# CLI installation

Requires **Python 3.10+**.

## Install from the repository

```bash
cd sdk/python
pip install -e ".[dev]"
```

Or:

```bash
cd sdk/python
pip install .
```

## Verify

```bash
acugis-report --version
acugis-report --help
```

## CI install example

```yaml
- uses: actions/setup-python@v5
  with:
    python-version: "3.12"
- name: Install CLI
  run: pip install ./sdk/python
```

## Next

- [Authentication](authentication.md)
- [Profiles](profiles.md)
