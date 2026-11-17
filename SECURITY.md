# Security policy

File Browser 3 is ToolsLib's independent fork of File Browser. The November 2026 review and remediation are tracked in [fix status](docs/reviews/fix-status.md). Fork releases and images are published through https://github.com/ToolsLib/filebrowser3 and Docker Hub `toolslib/filebrowser3`; upstream artifacts do not include these changes.

Report newly discovered vulnerabilities privately through [GitHub private vulnerability reporting](https://github.com/ToolsLib/filebrowser3/security/advisories/new). Do not put unpatched vulnerability details, credentials or private files in public issues. Include the exact version/commit/image digest, affected configuration, reproducible steps using temporary data and expected versus observed behavior.

The initial supported line is the 1.0.0 release-candidate series. Release candidates are intended for evaluation with backups and have not yet established a long-term support policy. Security updates will be published through this fork's Releases and advisories. Do not assume that an upstream advisory or release describes this fork's current behavior.

The command runner and hooks remain trusted shell-equivalent functionality and are disabled by default. Filesystem roots and their ancestors must be operator-controlled. The explicit external-symlink option removes confinement and should remain disabled. Authentication uses one server process per Bolt database; Redis upload coordination does not make independent account/session databases consistent. See [sessions](docs/sessions.md), [request limits](docs/request-limits.md) and [deployment](docs/deployment.md) for supported boundaries.

Local validation and resolved review findings are evidence for the tested configurations, not a guarantee that all possible defects have been found. The completion report records the checks and remaining operational limitations.
