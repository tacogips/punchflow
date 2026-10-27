# punchflow

## Go TLS security baseline

Go modules in this repository require Go 1.27.3 or newer for the eMudhra intermediate-certificate
mitigation (CVE-2026-111943). Rebuild existing binaries and container images to
activate the updated `crypto/x509` verifier.
