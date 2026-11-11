# Security validation record

On 11 November 2026 the maintained source was scanned with govulncheck v1.9.0
against the official Go vulnerability database last updated at 02:51:11 UTC.
The scanner used the actual Go 1.27.4 compiler on PATH, not a version override.

The final symbol scan of `./...` reported **no reachable or package-level known
advisories**. One module-only advisory remains: GO-2026-5932 in the unused
OpenPGP packages distributed inside `golang.org/x/crypto v0.58.0`. These packages
are neither imported nor reachable by this module. The stable dependency was
retained; the warning is recorded rather than hidden by an unneeded pseudo-version.

The original Go 1.25.1 build reached 31 standard-library advisories through
HTTP/TLS/X509/network paths. The module now requires Go 1.27.4 or newer and uses
updated stable cryptography/system packages. See `go.mod` for exact versions and
[raw scan records](security/) for the retained evidence.

Reproduce on the intended target platform and compiler:

```sh
go version
go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...
```

Known-advisory scans do not validate a custom wire protocol. The repaired v2
security layer has tests for the concrete authentication, replay, nonce,
plaintext-bypass and tampering defects found in the older source. Independent
cryptographic review, deployment abuse controls, and evaluation against established
authenticated transports remain outstanding before public game deployment.

Use unique, short-lived admission secrets issued over authenticated HTTPS.
`SecureTransport.PeerKeyID` exposes identity only for an authenticated active
session; application ownership/authorization checks remain the host's responsibility.
Do not treat a shared secret embedded in a distributed game client as player identity.
