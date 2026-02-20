# Story: Job Metadata (Labels and Annotations)

**Epic:** [epic02-jobbuilder](README.md)
**Priority:** Medium
**Status:** Not Started
**Estimated Effort:** 30 minutes

---

## User Story

As a **developer**, I want the Job to carry labels and annotations encoding the finding
context so operators can identify Jobs by fingerprint, kind, and parent without reading
environment variables.

---

## Acceptance Criteria

- [ ] Labels: `app.kubernetes.io/managed-by: mendabot-watcher`,
  `remediation.mendabot.io/fingerprint: <first-12>`,
  `remediation.mendabot.io/remediation-job: <rjob.Name>`,
  `remediation.mendabot.io/finding-kind: <kind>`
- [ ] Annotations: `remediation.mendabot.io/fingerprint-full: <full-64>`,
  `remediation.mendabot.io/finding-parent: <parentObject>`
- [ ] OwnerReference: points at the `RemediationJob` with correct UID, `Controller=true`,
  `BlockOwnerDeletion=true`
- [ ] Job settings: `BackoffLimit=1`, `ActiveDeadlineSeconds=900`,
  `TTLSecondsAfterFinished=86400`, `RestartPolicy=Never`
- [ ] Unit tests verify each label, annotation, ownerReference field, and job setting

---

## Tasks

- [ ] Write tests first (TDD)
- [ ] Implement metadata and job settings in `Build()`
- [ ] Confirm full `Build()` output is a valid `*batchv1.Job`

---

## Dependencies

**Depends on:** STORY_06 (volumes)
**Blocks:** Nothing — this completes the jobbuilder epic

---

## Definition of Done

- [ ] All tests from the JOBBUILDER_LLD.md test table pass with `-race`
- [ ] `go vet` clean
