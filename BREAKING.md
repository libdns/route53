# Breaking Changes

## Version 1.6 (Upcoming)

### libdns 1.0 Compatibility

Version 1.6 requires **libdns v1.0** or later. The libdns v1.0 release introduced typed record structs that replace the generic `libdns.Record` type. This is a fundamental change to the libdns API.

#### What Changed in libdns 1.0

- **Typed Records**: Instead of using generic `libdns.Record` structs, libdns v1.0 introduced typed record implementations like `libdns.Address`, `libdns.TXT`, `libdns.SRV`, etc.
- **Parse() Method**: The new `Record` interface includes a `Parse()` method that returns typed structs
- **RR() Method**: All record types implement `RR()` to get the underlying resource record data

#### Migration for libdns 1.0

See the [libdns documentation](https://pkg.go.dev/github.com/libdns/libdns) for complete details on migrating to typed records.

Example of the new API:
```go
// Old (libdns <1.0)
records := []libdns.Record{
    {
        Type:  "A",
        Name:  "www",
        Value: "1.2.3.4",
        TTL:   300 * time.Second,
    },
}

// New (libdns >=1.0)
records := []libdns.Record{
    libdns.Address{
        Name:  "www",
        Value: netip.MustParseAddr("1.2.3.4"),
        TTL:   300 * time.Second,
    },
}
```

### Field Renames

Two provider configuration fields have been renamed for clarity:

#### 1. `MaxWaitDur` → `Route53MaxWait`

**Old (pre-v1.6):**
```go
provider := &route53.Provider{
    MaxWaitDur: 60,  // Was treated as seconds (multiplied by time.Second internally)
}
```

**New (v1.6+):**
```go
provider := &route53.Provider{
    Route53MaxWait: 60 * time.Second,  // Use proper time.Duration
}
```

**Important:** In versions before v1.6, `MaxWaitDur` was silently multiplied by `time.Second` in the provider's init function. This was non-idiomatic Go and has been fixed. You must now provide a proper `time.Duration` value (like `60 * time.Second` or `2 * time.Minute`), as is standard in Go.

**Failure to multiply by `time.Second` will result in a 60-nanosecond timeout instead of 60 seconds!**

**Rationale:** The new name clearly indicates this is a Route53-specific timeout for AWS internal propagation, not general DNS propagation.

#### 2. `WaitForPropagation` → `WaitForRoute53Sync`

**Old (pre-v1.6):**
```go
provider := &route53.Provider{
    WaitForPropagation: true,
}
```

**New (v1.6+):**
```go
provider := &route53.Provider{
    WaitForRoute53Sync: true,
}
```

**Rationale:** The new name clearly indicates this waits for Route53's internal synchronization, not worldwide DNS propagation (which can take hours depending on TTL values).

### JSON Configuration

If you're using JSON configuration, update your field names:

**Old (pre-v1.6):**
```json
{
  "max_wait_dur": 60,
  "wait_for_propagation": true
}
```

**New (v1.6+):**
```json
{
  "route53_max_wait": "2m",
  "wait_for_route53_sync": true
}
```

Note: The old `max_wait_dur` field accepted a numeric value (seconds), while the new `route53_max_wait` field uses standard Go duration strings like `"2m"`, `"120s"`, etc.

## Migration Checklist

- [ ] Update to libdns v1.0+ (see libdns documentation for typed records)
- [ ] Rename `MaxWaitDur` to `Route53MaxWait` in your code
- [ ] Change from plain integer (e.g., `60`) to proper `time.Duration` (e.g., `60 * time.Second`)
- [ ] Rename `WaitForPropagation` to `WaitForRoute53Sync` in your code
- [ ] Update JSON/YAML configuration files with new field names
- [ ] Test your code thoroughly after migration
