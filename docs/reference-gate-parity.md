# Parity with the reference gate

The gate this repository is held to is the one already running on
[iderex/jellyfin-plugin-sso](https://github.com/iderex/jellyfin-plugin-sso). It
is public, its ruleset is readable by anybody, and it is a standard that exists
rather than one invented here.

Parity is not copying. That gate protects a plugin loaded into somebody else's
process. This is a network service that terminates untrusted connections and
carries personal communications. Some of the reference does not apply, and some
of what a service needs is not there at all. Both directions are deviations and
both owe a reason, so every row below carries one.

## The two rulesets, read rather than described

Run these yourself. A ruleset is repository settings, settings move, and this
document cannot tell you when they have.

    gh api repos/iderex/jellyfin-plugin-sso/rulesets --jq '.[] | select(.name=="Protect main and 5.0") | .id'
    18802863
    gh api repos/iderex/jellyfin-plugin-sso/rulesets/18802863 --jq '{enforcement, bypass: .bypass_actors, required: [.rules[] | select(.type=="required_status_checks") | .parameters.required_status_checks[].context]}'
    {"bypass":[],"enforcement":"active","required":["build","ABI floor build","Package (JPRM) / Build package","Package (JPRM) / Generate SBOM","CodeQL","Analyze (csharp)","DCO sign-off","Deterministic PR-hygiene checks","Enforce greppable invariants","Reject Trojan Source Unicode","Audit workflows (zizmor)","prettier","dependency-review"]}

    gh api repos/iderex/stammtisch/rulesets/20482339 --jq '{enforcement, bypass: .bypass_actors, types: [.rules[].type]}'
    {"bypass":[],"enforcement":"active","types":["deletion","non_fast_forward","pull_request"]}

Both were run on 2026-08-06 against the commit this document lands on.

The second output is the whole distance in one line. This repository's ruleset
carries no `required_status_checks` entry at all, so every workflow here is
advisory: a pull request with a red check merges exactly like a green one. Issue
[#25](https://github.com/iderex/stammtisch/issues/25) is where the list is
requested, and until it is applied, everything below describes checks that run
and nothing that refuses.

## Required checks on the reference, element by element

| Reference check | Here | Reason | Delivered by |
| --- | --- | --- | --- |
| `build` | Adopt | A change that does not compile is refused before a person reads it, and that is language-independent. | [#15](https://github.com/iderex/stammtisch/issues/15), landed |
| `ABI floor build` | Drop | It proves a plugin still loads into the oldest host it claims to support. This ships its own binary and loads into nothing, so the check has no subject. | none |
| `Package (JPRM) / Build package` | Adapt | The packaging artefact there is a plugin archive for a specific host. Here it is a container image, which is how this software actually reaches an operator. | [#87](https://github.com/iderex/stammtisch/issues/87) |
| `Package (JPRM) / Generate SBOM` | Adopt | An operator asking whether a published vulnerability touches what they run needs the list, and generating it at release time means the generator's first run is the day it matters. | [#19](https://github.com/iderex/stammtisch/issues/19) |
| `CodeQL` | Adopt | Dataflow analysis over the server source. A service that parses untrusted frames needs it more than a plugin does, not less. | [#20](https://github.com/iderex/stammtisch/issues/20) |
| `Analyze (csharp)` | Adapt | The same element as the row above with a different language pack. It is one check here rather than two, because there is one server language. | [#20](https://github.com/iderex/stammtisch/issues/20) |
| `DCO sign-off` | Adopt | Contributor origin has to be asserted per commit whatever the software does. | landed with the first commit; the certificate text it points at came from [#21](https://github.com/iderex/stammtisch/issues/21) |
| `Deterministic PR-hygiene checks` | Adopt | The contribution guide states things about what a change carries and today nothing reads any of it. | [#77](https://github.com/iderex/stammtisch/issues/77) |
| `Enforce greppable invariants` | Adopt | The invariants differ because the code differs, but the mechanism of pinning a rule to a pattern a person can run is the same. | [#76](https://github.com/iderex/stammtisch/issues/76) |
| `Reject Trojan Source Unicode` | Adopt | CVE-2021-42574 is about how source renders to a reviewer, which has nothing to do with what the source does. | landed with the first commit |
| `Audit workflows (zizmor)` | Adopt | Workflow YAML runs with a token and can be triggered from outside, so it is audited like any other code. | landed with the first commit |
| `prettier` | Adapt | One formatter per toolchain rather than one formatter. `gofmt` covers the server. The client's formatter cannot be chosen before the client platform is. | server half landed with [#17](https://github.com/iderex/stammtisch/issues/17); client half waits on [#58](https://github.com/iderex/stammtisch/issues/58) |
| `dependency-review` | Adopt | A known vulnerability arriving with a new dependency is refused at the moment it is introduced, which is the cheapest moment. | landed with the first commit |

Thirteen contexts, twelve elements, because `CodeQL` and `Analyze (csharp)` are
the same job seen twice in that list.

## What the reference runs without gating on it

| Reference element | Here | Reason | Delivered by |
| --- | --- | --- | --- |
| Coverage bar pinned on the security-deciding modules rather than on everything | Adapt | The bar is right and the surface it is pinned to is different: there it is the login path, here it is admission, permission and protocol decoding. | [#72](https://github.com/iderex/stammtisch/issues/72) |
| Mutation testing, reporting rather than gating | Adopt | A mutation score turned into a merge condition becomes a number people manage. | [#73](https://github.com/iderex/stammtisch/issues/73) |
| Fuzzing on a schedule | Adopt | Wider here than there, because this parses untrusted frames off the network rather than configuration written by an administrator. | [#74](https://github.com/iderex/stammtisch/issues/74) |
| End-to-end harness | Adapt | There it drives a login. Here it has to boot a media plane and prove audio arrived, which is a different order of expense and is why it runs nightly and at a release. | [#75](https://github.com/iderex/stammtisch/issues/75) |
| Scorecard supply-chain self-audit | Adopt | Unchanged. It scores the repository rather than the software. | landed with the first commit |

That the coverage bar on the reference is a pinned number rather than a mood is
worth reading in the source rather than in this sentence:

    gh api repos/iderex/jellyfin-plugin-sso/contents/scripts/check-coverage.py --jq .content | base64 -d | grep -n 'SECURITY_LINE_BAR'
    68:SECURITY_LINE_BAR = 92.0
    125:    print(f"Security-surface bar (pinned):  {SECURITY_LINE_BAR:.1f}%")
    127:    if security < SECURITY_LINE_BAR:
    130:            f"{SECURITY_LINE_BAR:.1f}% bar (#718). Cover the security-decision paths you touched, "

## What this repository adds that the reference does not have

Each of these is a deviation upward and owes its reason as much as a dropped
check does.

| Element | Reason | Delivered by |
| --- | --- | --- |
| A unit suite that must pass headless and without elevation | The reference runs in one place. This has to run on a bare runner and on a maintainer's machine without either being asked to grant anything. | [#16](https://github.com/iderex/stammtisch/issues/16) |
| A deterministic test harness, with time and identifiers injected | A service with timeouts, reconnects and grace periods produces a suite that fails once a fortnight and teaches people to re-run it. | [#24](https://github.com/iderex/stammtisch/issues/24) |
| A locked dependency graph with a reproducible restore | The reference has a lockfile because its ecosystem produces one. Here it is a thing to build rather than a thing to keep. | [#18](https://github.com/iderex/stammtisch/issues/18) |
| A mouth-to-ear measurement bench, and a latency budget pinned from it | Nothing in the reference has a latency claim to keep. Every user-visible promise this project makes is a number, so the bench is a gate input rather than a curiosity. | [#4](https://github.com/iderex/stammtisch/issues/4), [#6](https://github.com/iderex/stammtisch/issues/6) |
| A media integration harness, gated on hardware | Real media cannot be proved by a stub, and the harness has to declare when it did not run rather than passing quietly. | [#45](https://github.com/iderex/stammtisch/issues/45) |
| Forwarding delay measured against the budget | The budget is only a budget if something reads it. | [#46](https://github.com/iderex/stammtisch/issues/46) |
| The idle cost of always-on rooms | This project encourages the configuration nobody measures, so it owes the number for it. | [#47](https://github.com/iderex/stammtisch/issues/47) |
| Behaviour under loss and constrained bandwidth | A plugin degrades by being slow. A voice service degrades by becoming unusable while still reporting healthy. | [#48](https://github.com/iderex/stammtisch/issues/48) |
| Reachability for an operator behind a home router | The most common self-hosting failure is a service that starts perfectly and cannot be reached. | [#49](https://github.com/iderex/stammtisch/issues/49) |
| Load and soak evidence | A plugin holds no long-lived connections. This holds thousands, and the failures that matter appear on the second day rather than in the first minute. | [#94](https://github.com/iderex/stammtisch/issues/94) |
| A measured footprint on a named machine | An operator deciding whether to add this to their stack needs a number, and every project in this category answers with a shrug. | [#89](https://github.com/iderex/stammtisch/issues/89) |
| Verified signatures on the protected branch | The reference does not require them; its ruleset `types` list above carries no `required_signatures`. Asking for them here is a deviation upward and is argued on its own issue rather than assumed. | [#78](https://github.com/iderex/stammtisch/issues/78) |

## What this document does not claim

It reads two rulesets and one script from the reference and reports what they
say. It does not claim that the reference gate is complete, that this list is
the whole of what the reference does, or that adopting every row would make the
two repositories equally well protected.

Nothing refuses a change to this document that contradicts either repository.
The rows naming issues were checked against the tracker on the day this landed,
by number, and a row naming an issue that is later renamed will go on naming it.
Re-run the commands rather than trusting the outputs pasted above.
