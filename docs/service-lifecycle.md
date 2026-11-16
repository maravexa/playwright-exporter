# Service lifecycle and isolation

The exporter runs operator-managed test code. The Go process, Playwright workers,
browsers and custom commands share the service account and its access. This is
not a sandbox for untrusted test projects, npm packages or custom commands, and
it does not isolate one schedule from another.

## Packaged systemd service

The unit uses an unprivileged account, `NoNewPrivileges=yes`, a read-only system
view, protected home directories and private temporary storage. `PrivateTmp=yes`
provides writable `/tmp` inside that service namespace. The unit does not require
a pre-existing host `/tmp/playwright-exporter` directory. Existing directories
from older package installations are left alone.

`KillMode=control-group` and `SendSIGKILL=yes` make service stop/restart apply to
all remaining processes in the service cgroup. Systemd sends SIGTERM to that
entire cgroup, not just the exporter. The exporter's own shutdown joins its
schedule goroutines and interrupts direct runners, but the simultaneous SIGTERM
can preempt their graceful browser cleanup. Do not rely on every teardown hook
running during service stop. If the service has not stopped within
`TimeoutStopSec=10`, systemd force-kills the remaining cgroup members. Never
configure `KillMode=process` or disable the final kill if relying on this stop
behavior. Processes must not have permission to move out of their service cgroup.

An individual schedule timeout is different: it cancels the direct CLI and
bounded output capture, but does not stop the entire service. Arbitrary custom
commands can detach children that outlive that run. There is no per-run cgroup,
privileged cleanup broker or same-UID hostile-code boundary. The service memory
limit is shared by all schedules. Do not claim hard per-run containment from
cooperative CLI cleanup tests.

## Browser sandbox and deployment

Keep the browser's own sandbox enabled when the deployment requires that
boundary. Playwright projects control browser launch options; the exporter does
not enforce them. A successful journey alone does not prove that the sandbox
was enabled. Validate the actual project, browser version, operating-system
policy and service identity before deploying.

Do not disable AppArmor, the host's user-namespace restrictions, container
seccomp or the browser sandbox to make a smoke test pass. A necessary policy
exception needs a separate review and approval for that exact deployment. A
temporary executable-path exception in a user-writable scratch directory is not
a production policy or a hash-bound authorization.

## Container process supervision

The image runs standard Tini as PID 1 under the same non-root service identity.
Its default signal forwarding targets only the Go child, preserving the
exporter's cooperative runner cancellation. Tini also reaps adopted dead
children while the exporter remains running; Go is not used as a general
orphan reaper. Do not enable Tini process-group forwarding if relying on that
signal sequence.

This addresses zombie accumulation, not arbitrary live detached descendants or
per-run isolation. Container stop removes the container's remaining processes;
individual timeouts still have the cooperative limits described above. The
host package continues to use systemd rather than adding another host service.

## Local release checks

- Validate the exact packaged unit with `systemd-analyze verify` and confirm it
  starts under the intended non-root service identity with `PrivateTmp` active.
- Run the real version-matched CLI and browser against a local fixture. Verify
  expected DOM results, cancellation hooks and worker/browser termination.
- Check service stop with detached, TERM-ignoring fixtures in the same cgroup.
  Verify the recorded process identities and cgroup are gone after the deadline.
- Recheck the final image's Node, Playwright, browser and OS inventory, package
  advisories, non-root execution and project dependency compatibility. The Go
  and npm advisory workflows do not cover this complete image.
- Restore temporary test policy and stop all fixtures. Production deployment,
  signed release publication and remaining security findings are separate gates.

## Opt-in single-schedule failure boundary (candidate)

`restart_on_run_failure: true` requires exactly one configured schedule. A runner
execution, timeout, capture or report-parsing error stops the exporter with exit
status 1, after bounded shutdown. An external service manager must then stop
all remaining processes in that instance's cgroup before restarting it. Normal
SIGTERM/SIGINT remains a successful shutdown. A valid Playwright report with
failed assertions is an ordinary monitoring result, not a restart trigger.

This mode is disabled by default. It does not create a cgroup, acquire manager
privileges, verify policy, or isolate code by itself. Use separately scoped
service/container instances and separate accounts/project paths when schedules
must not share access. The existing `Restart=on-failure`, whole-cgroup kill and
final-kill settings are prerequisites to the intended native deployment; they
must be verified on the loaded unit. Restart limits and backoff must prevent a
broken project from creating an unbounded restart loop.

Metrics are in memory: they disappear during restart, and the last failure
metric may never be scraped. Alert on exporter availability and staleness as
well as test results. This mode only improves the failure cleanup boundary.
A successful command can still leave live descendants until the next unit stop;
it is not complete per-run cleanup or an untrusted-code sandbox. Hard cleanup
on every completed run requires a separately supervised one-shot worker model.

Configuration now rejects unknown YAML fields and extra documents, including
misspelled restart-mode settings. Existing configurations with ignored fields
must be corrected before upgrade. Explicit negative timeouts and defaults that
round to zero are also rejected.

In the VM-only benign fixture test, a one-second configured timeout was followed
by five seconds of direct-process grace, up to one second of output draining,
and the test unit's five-second final-kill budget. The configured run timeout
is not itself the deadline by which every descendant is gone. Account for the
whole cleanup budget when choosing restart and alert timing.

## Opt-in per-run PID namespace cleanup (unqualified candidate)

`isolate_process_tree: true` asks Linux to launch each direct runner as PID1 in a
new PID namespace nested in an unprivileged user namespace. The only host UID/GID
mapped is the exporter's current identity; no cgroup or service-manager authority
is acquired. Linux kills remaining namespace processes when its PID1 exits,
including children in other sessions. This provides the mechanism for cleanup on
successful completion as well as timeout. Permission denial is a failed run,
never an unisolated retry. Non-Linux configurations reject the option.

This is not a filesystem/network/credential isolation boundary. The project still
has the service identity's access, and namespaces add kernel attack surface.
Browser startup and nested sandbox compatibility require separate qualification.
PID1 signal behavior can affect teardown; direct runner force-kill remains bounded
by cancellation grace. Exporter crashes still depend on external whole-service
cleanup because no parent-death guarantee is claimed here.

Default tests validate attribute construction and no retry. Real namespace success
and timeout acceptance is separately gated by
`PLAYWRIGHT_APPROVED_PID_NAMESPACE_TEST=1`; skipped is not passed. Host AppArmor and
namespace policy must be reviewed before that test. Do not disable global controls
or claim this candidate is production-qualified.
