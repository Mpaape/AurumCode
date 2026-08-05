Fixture: FIXED
Rule: SCR-005 — Software and Data Integrity Failures (CWE-502)

The session is verified against a MAC computed at issuance before it is
decoded at all, and decoding is bounded to one known concrete type.

```go
func LoadSession(data, mac []byte, key []byte) (*Session, error) {
    if !hmac.Equal(mac, computeMAC(data, key)) {
        return nil, errors.New("session integrity check failed") // fail closed
    }
    var s Session
    dec := json.NewDecoder(bytes.NewReader(data))
    dec.DisallowUnknownFields()
    if err := dec.Decode(&s); err != nil {
        return nil, err
    }
    return &s, nil
}
```

Tampered or unsigned data never reaches the decoder at all.
