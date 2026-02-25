# raw/

Raw tool output for this review. Files in this folder are created during the review
and committed alongside the phase files.

## Expected files

| File | Created by | Phase |
|------|-----------|-------|
| `govulncheck.txt` | `govulncheck ./... > raw/govulncheck.txt` | Phase 1 |
| `gosec.json` | `gosec -fmt json -out raw/gosec.json ./...` | Phase 1 |
| `staticcheck.txt` | `staticcheck ./... > raw/staticcheck.txt` | Phase 1 |
| `trivy-agent.txt` | `trivy image ... mendabot-agent:review-scan > raw/trivy-agent.txt` | Phase 8 |
| `trivy-watcher.txt` | `trivy image ... mendabot-watcher:review-scan > raw/trivy-watcher.txt` | Phase 8 |
| `watcher-audit.txt` | `kubectl logs ... > raw/watcher-audit.txt` | Phase 7 |

Add any other raw output files here as needed.
