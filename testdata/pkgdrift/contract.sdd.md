## Contract

```sdd
package: pkgdrift
exports:
  - name: Value
    kind: func
    signature: "func Value() string"
scenarios:
  - id: one
    when: call
    then: returns 1
```
