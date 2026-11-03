# CLI resources

```bash
acugis-report resources list
acugis-report resources upload logo.png --folder-id 3
acugis-report resources download <uuid> --output logo.png
acugis-report resources replace <uuid> new-logo.png
acugis-report resources delete <uuid> --yes
acugis-report resources images
```

Downloads are written to files — avoid printing binary content to the terminal.

See [Server resources](../../user-guide/resources.md).
