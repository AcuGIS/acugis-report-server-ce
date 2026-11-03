# Licensing

Report Server is published as two packages. A license file turns Professional features on inside the Professional package. `ACUGIS_EDITION` does not.

| Runtime | What you installed | What runs |
|---------|--------------------|-----------|
| Open Source package | `acugis-report-server` | Professional modules are not in the binary. A license file cannot add them. |
| Professional package, no valid license | `acugis-report-server-professional` | The process starts. The effective edition stays the community allow-list. The UI name is **Open Source**. Denials say **Community Edition**. |
| Professional package, valid license | same package, after activation | Professional features in that binary are entitled until the license expiry. |

The feature list is the [compatibility matrix](../reference/compatibility-matrix.md). This page is how activation works.

Report Designer Web is a different product. A Report Server Professional license does not enable it. See [Designer Web](designer-web.md).

## Before activation

The Professional package starts without a license. Reports, datasources, folders, resources, local login, and personal access tokens work. Data Explorer and the community map allow-list are available because they are on that allow-list. Dashboards, Map Reports, Dataset Intelligence, MCP, audit administration, usage analytics, governance, audit export, OIDC, SAML, and ArcGIS stay off until the license is valid.

The Open Source package never reaches the third row. Its Administration → License page says to install AcuGIS Report Server Professional. Trial and activation requests on that package are rejected with “This package cannot enable Report Server Professional.”

## Administration → License

Platform administrators open **Administration → License** at `/admin/license`. The same actions exist as JSON for an authenticated platform administrator:

| Method | Path | Action |
|--------|------|--------|
| `GET` | `/api/v1/admin/license` | Report Server license status |
| `POST` | `/api/v1/admin/license/activate` | Body `{"activation_key":"…"}` |
| `POST` | `/api/v1/admin/license/refresh` | Refresh the stored license |
| `POST` | `/api/v1/admin/license/trial` | Start a trial when the license service allows it |

The page has a separate Report Designer Web section. Those calls use `/api/v1/admin/license/designer` and the `activate`, `refresh`, and `trial` paths under it. Until Designer Web is configured, that section shows **Not installed** and does not offer activation. See [Designer Web](designer-web.md).

The status view shows the installed package capability (`professional` or `community`), the effective edition, whether a license file is present, whether it is currently valid, the installation ID, product code, expiry, and the last refresh result. It does not include the activation key or the stored token.

## Activation

1. Install the Professional package and sign in as a platform administrator.
2. Open **Administration → License**.
3. Enter the activation key and activate, or start a trial when the page offers one.

The server sends the key and the installation ID to the license service. The default base URL is `https://license.acugis.com`. Set `ACUGIS_LICENSE_SERVER_URL` only to override that base, and only with an `https` URL. The activation key is not written to disk.

On success the service returns a license token. The server verifies it locally and stores it as `license.lic`. Professional features apply immediately. A restart is not required for the edition check.

A trial is available only when the license service reports that this installation is eligible. For a normal available offer, the button uses the day count the service returns. If the service reports an already-active trial and does not return a positive day count, the button label uses 14 days so that existing trial can be installed. That 14 is only the label for that recovery case. It is not a local trial length, and the service remains authoritative for a normal offer. The Open Source package does not offer the Professional trial. A trial that has already been used on that installation is not offered again.

## Installation ID and license files

The state directory is `ACUGIS_LICENSE_STATE_DIR`.

| Deployment | Directory |
|------------|-----------|
| Default, including the Debian package and tarball | `/var/lib/acugis-report-server` |
| Docker Compose | `/opt/acugis-report-server/var/license` on the `report-resources` volume |

| File | Contents |
|------|----------|
| `installation-id` | A random installation UUID created on first use. It is not derived from the hostname, MAC address, or IP address. A valid file is never replaced. A malformed file is left in place. License status becomes `identity_invalid`, with the message that the identity file is invalid and was not replaced. |
| `license.lic` | The Report Server Professional token, after local verification. |
| `report-designer-web.lic` | The Report Designer Web token, when that product is activated. It does not entitle Report Server Professional. |

Designer Web reads the existing `installation-id`. It does not create a second installation identity.

Back up this directory with the rest of the host state. See [Backups](backups.md). The files are credentials. Restrict who can read them.

## Invalid, expired, and missing licenses

| State | Effective edition |
|-------|-------------------|
| No `license.lic` | Community allow-list. The server keeps running. |
| Token expired | Community allow-list. Status is expired. |
| Token for another product | Not entitled. Status is wrong product. |
| Token for another installation ID | Not entitled. Status is installation mismatch. |
| Malformed `installation-id` | Not entitled. Status is `identity_invalid`. The file is not replaced. |
| Token fails verification | Not entitled. Status is `invalid`. |
| Open Source package, even with a Professional token on disk | Not entitled. The page says this package cannot enable Report Server Professional. |
| License service unreachable while the local token is still inside its expiry | Professional stays on until `expires_at`. Refresh can be retried later. |
| Refresh denied or the replacement token is rejected | The previous valid token remains until it expires. |

After expiry, Professional features stop. Data Explorer remains available on the Professional package because it is on the community allow-list. Features that are not on that list stop.

## Upgrades

`apt-get install` of a newer `.deb`, and re-running `install.sh`, keep `/etc/acugis-report-server/env` and do not rewrite the license state directory. `JRS_APP_VERSION` in the env file is refreshed. Replacing the Open Source package with the Professional package does not by itself activate a license. Activate on **Administration → License** after the package change.

`apt remove` and `apt purge` stop the services and do not delete the license directory.

## Environment variables

| Variable | Role |
|----------|------|
| `ACUGIS_LICENSE_STATE_DIR` | State directory. Default `/var/lib/acugis-report-server`. |
| `ACUGIS_LICENSE_SERVER_URL` | Optional HTTPS base URL. Empty uses `https://license.acugis.com`. |
| `ACUGIS_EDITION`, `JRS_EDITION` | Diagnostics only. |

Full table: [Environment variables](../reference/environment-variables.md).

## Related

- [Compatibility matrix](../reference/compatibility-matrix.md)
- [Upgrades](upgrades.md)
- [Designer Web](designer-web.md)
- [Backups](backups.md)
- OpenID Connect and SAML are available in Professional and are not part of this Open Source documentation.
