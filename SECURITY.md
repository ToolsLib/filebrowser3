# Security policy

This checkout is a local fork of archived upstream File Browser. The November 2026 review and its remediation are tracked in [fix status](docs/reviews/fix-status.md). Upstream release binaries and Docker images do not contain these local changes. No public release, remote repository or external security reporting channel is configured for this fork yet.

Report a newly discovered issue privately to the operator or maintainer of the deployment you are using. Include the exact local commit, affected configuration, reproducible steps using temporary data, and expected versus observed behavior. Review upstream advisories for inherited behavior, and distinguish the code in this fork from upstream releases.

The command runner and hooks remain trusted shell-equivalent functionality and are disabled by default. Filesystem roots and their ancestors must be operator-controlled. The explicit external-symlink option removes confinement and should remain disabled. Authentication uses one server process per Bolt database; Redis upload coordination does not make independent account/session databases consistent. See [sessions](docs/sessions.md), [request limits](docs/request-limits.md) and [deployment](docs/deployment.md) for supported boundaries.

Local validation and resolved review findings are evidence for the tested configurations, not a guarantee that all possible defects have been found. The completion report records the checks and remaining operational limitations.
