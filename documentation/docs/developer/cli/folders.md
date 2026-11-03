# CLI folders

```bash
acugis-report folders tree
acugis-report folders browse --folder-id 0
acugis-report folders create Finance
acugis-report folders update 12 --name Billing
acugis-report folders move 12 --parent-id 3
acugis-report folders delete 12 --yes
acugis-report folders permissions 12
```

Export the tree for review:

```bash
acugis-report --output yaml folders tree > folders.yaml
```

See [Server folders](../../user-guide/folders.md) for the underlying REST surface.
