# Route53 Provider Examples

Set up AWS credentials using environment variables:
```bash
export AWS_ACCESS_KEY_ID=your-access-key
export AWS_SECRET_ACCESS_KEY=your-secret-key
export AWS_REGION=us-east-1  # optional
```

Always use fully qualified zone names with a trailing dot (e.g., `example.com.`)

## Examples

**List all records:**
```bash
cd list && go run . example.com.
```

**Create TXT records:**
```bash
cd createTxt && go run . example.com.
```

The provider returns typed structs like `libdns.Address`, `libdns.TXT`, etc., with specific fields for each record type.