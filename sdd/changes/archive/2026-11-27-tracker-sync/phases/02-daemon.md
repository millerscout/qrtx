# Phase sync daemon

- [x] 2.1 Add `tryLockFile` (Unix: `Flock` with `LOCK_EX|LOCK_NB`; Windows: one `O_EXCL` attempt that writes `pid nonce`, refresh and remove only after checking the nonce, and the 60s stale rule), and `daemonRunning(root)`. Make the Windows `lockFile` write and check a nonce the same way, and make `DrainOutbox` re-take the outbox lock per entry (D14). verify: `go test . -run TestDaemonLock` shows a second `tryLockFile` failing while the first is held and succeeding after release. covers S7
{: id="phase-02-task-1"}
- [x] 2.2 Add `sdd sync serve`, the daemon loop:
  - **Start:** take `daemon.lock` non-blocking (exit 0 if it is held), delete any stale `stop` file, write `daemon.json`, and trim the event log under the sync lock per Q13.
  - **Each tick:** expire leases through `withSync`, and collect the live roots. Drains (`DrainOutbox` per root, with `OpenTracker(root)`) run in a goroutine and are skipped while the previous one runs. The sync lock is never held during them.
  - **Exit:** when no lease is live, drain once more, then under the sync lock re-check for live leases, release `daemon.lock`, log `exiting: no live sessions` and exit 0.
  - **Stop:** a `stop` file in the state dir triggers the same exit path. `sdd sync stop` writes it only while `daemonRunning`, and waits up to 5s.
  - **Logging:** to `sync.log`, rotated at 1 MB.

  verify: `go test ./cli/ -run TestDaemon` (built `sdd` binary, `sync.lease: 2s`, `sync.tick: 200ms`, and a cleanup helper that runs `sync stop` and fails if the pid survives) covers the S8 exit, the S9 drain of a fake plugin's pending write, the S12 stop, the S14 trim, and S20 with two subproject roots. covers S8,S9,S12,S14,S20
{: id="phase-02-task-2"}
- [x] 2.3 Make `heartbeat`, `begin` and `claim` start the daemon when `daemonRunning` is false (checked under the sync lock, Q7), unless `CI`, `--no-daemon` or `SDD_NO_DAEMON=1`. The daemon runs as `os.Executable() sync serve`, detached per D3. verify: `go test ./cli/ -run TestDaemonAutostart` covers two concurrent heartbeats leaving one daemon, `CI=true` starting none while still updating the lease, a killed daemon being replaced by the next heartbeat, and the S22 exit race repeated 20 times. covers S7,S22
{: id="phase-02-task-3"}
- [x] 2.4 Add `sdd sync status [--json]` per Q11. verify: `go test ./cli/ -run TestDaemonStatus` checks the S12 JSON fields. covers S12
{: id="phase-02-task-4"}
