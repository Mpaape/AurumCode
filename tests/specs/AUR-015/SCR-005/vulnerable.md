Fixture: VULNERABLE
Rule: SCR-005 — Software and Data Integrity Failures (CWE-502)

Attacker-controlled bytes cross from an external channel into a
deserializer that can instantiate arbitrary registered types by default.

```go
func LoadSession(data []byte) (*Session, error) {
    var s Session
    dec := gob.NewDecoder(bytes.NewReader(data))
    if err := dec.Decode(&s); err != nil { // any registered type is allowed
        return nil, err
    }
    return &s, nil
}
```

A crafted payload can instantiate an unexpected concrete type or trigger
unbounded allocation during decode.
