# BlackChain Town v1 Incident Escalation Matrix

## Severity Levels

| Severity | Condition | Initial Response | Escalation Target | Target Response |
| --- | --- | --- | --- | --- |
| SEV-1 | Multi-node outage, consensus unavailable, or HTTPS control plane broadly unavailable | Freeze promotion and begin rollback assessment immediately | on-call operator + release owner | immediate |
| SEV-2 | Single-node failure, sustained sync divergence, repeated replay failures, or repeated readiness loss | Isolate affected node and begin field verification | on-call operator | within 15 minutes |
| SEV-3 | Non-blocking trust issue, warning condition, or documentation/operator tooling defect | Record issue and apply scheduled remediation | service owner | same business day |

## Escalation Roles

- on-call operator: executes verification, containment, and rollback steps
- release owner: owns release decision and rollback approval
- service owner: owns code/config follow-up and post-incident corrective action

## Required Incident Record

- timestamp
- severity
- affected nodes
- observed symptom
- verification evidence
- containment action
- rollback decision
- follow-up owner

