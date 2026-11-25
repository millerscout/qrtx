# pkgok test package

## Contract

```sdd
package: pkgok
exports:
  - name: Greet
    kind: func
    signature: "func Greet(name string) string"
scenarios:
  - id: greet
    when: name is non-empty
    then: returns greeting
```
