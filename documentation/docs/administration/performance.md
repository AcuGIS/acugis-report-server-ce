# Performance

## Practical levers

| Area | Guidance |
|------|----------|
| Jasper timeout | `JRS_JASPER_TIMEOUT` (default `600s`) — raise only when large fills need it |
| Report queries | Optimize JRXML SQL; avoid unbounded result sets |
| Explorer | Use row limits; prefer indexed columns for filters |
| Network | Keep Tomcat Jasper internal ports on the app tier; avoid routing fill traffic over the public internet |
| Resources | Prefer resource UUIDs and local fonts/drivers over remote fetches at fill time |

## Capacity notes

- Concurrent report fills are limited by Tomcat / Jasper and datasource capacity
- Explorer spatial analysis and large exports can be CPU- and memory-heavy — size the host accordingly
- Separate OLTP reporting replicas when interactive Explorer load competes with transactional databases

## Related

- [Configuration](configuration.md)
- [Environment variables](../reference/environment-variables.md)
- [Diagnostics](diagnostics.md)
