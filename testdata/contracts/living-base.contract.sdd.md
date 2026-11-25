# inferencegate contract

## Contract

```sdd
package: inferencegate
exports:
  - name: ErrBusy
    kind: var
    signature: "var ErrBusy"
  - name: Logf
    kind: var
    signature: "var Logf"
  - name: Gate
    kind: type
    signature: "type Gate"
  - name: New
    kind: func
    signature: "func New(enabled bool, maxQueue int, failFast bool) *Gate"
  - name: Init
    kind: func
    signature: "func Init(g *Gate)"
  - name: Global
    kind: func
    signature: "func Global() *Gate"
  - name: Settings
    kind: type
    signature: "type Settings"
  - name: SettingsFromEnv
    kind: func
    signature: "func SettingsFromEnv() Settings"
  - name: Snapshot
    kind: method
    signature: "func (g *Gate) Snapshot() (busy bool, waiters int)"
  - name: Acquire
    kind: method
    signature: "func (g *Gate) Acquire(ctx context.Context) (release func(), err error)"
  - name: NowFunc
    kind: method
    signature: "func (g *Gate) NowFunc(now func())"
  - name: IdleFor
    kind: method
    signature: "func (g *Gate) IdleFor() time.Duration"
  - name: UserWaiting
    kind: method
    signature: "func (g *Gate) UserWaiting() bool"
  - name: AcquireBackground
    kind: method
    signature: "func (g *Gate) AcquireBackground(ctx context.Context) (release func(), err error)"
scenarios:
  - id: single-slot
    when: gate is enabled and slot is free
    then: Acquire grants the slot until release runs
  - id: fail-fast-busy
    when: fail-fast mode is on and slot is held
    then: Acquire returns ErrBusy
  - id: idle-after-release
    when: a user acquisition is released and time passes
    then: IdleFor reports the time since that release
  - id: background-does-not-reset-idle
    when: a background acquisition runs after user activity
    then: IdleFor stays unchanged and a waiting user request is served first
```
