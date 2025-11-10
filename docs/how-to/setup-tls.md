# How to Set Up TLS for Relay

This guide walks you through configuring TLS encryption for incoming connections to the relay service.

## Why Use TLS?

TLS encryption protects sensitive log data (user activity, audit logs, etc.) transmitted from Zscaler ZPA LSS to your relay service. Without TLS, this data travels over the network in plain text.

## Prerequisites

- OpenSSL installed on your system
- Write access to the relay configuration directory
- Basic familiarity with TLS certificates

## Option 1: Self-Signed Certificates (Testing/Internal Use)

Self-signed certificates are suitable for testing environments or internal deployments where you control both endpoints.

### Generate a Self-Signed Certificate

```bash
# Generate private key and certificate valid for 365 days
openssl req -x509 -newkey rsa:4096 \
  -keyout relay-key.pem \
  -out relay-cert.pem \
  -days 365 \
  -nodes \
  -subj "/CN=relay.example.com/O=YourOrg/C=CA"
```

Parameters explained:
- `-x509`: Generate a self-signed certificate
- `-newkey rsa:4096`: Create a 4096-bit RSA private key
- `-keyout`: Output file for the private key
- `-out`: Output file for the certificate
- `-days 365`: Certificate validity period
- `-nodes`: Don't encrypt the private key (no passphrase)
- `-subj`: Certificate subject information

### Verify the Certificate

```bash
# View certificate details
openssl x509 -in relay-cert.pem -text -noout

# Check certificate and key match
openssl x509 -noout -modulus -in relay-cert.pem | openssl md5
openssl rsa -noout -modulus -in relay-key.pem | openssl md5
# The MD5 hashes should match
```

## Option 2: Let's Encrypt Certificates (Public Internet)

If your relay service is accessible from the public internet, you can use Let's Encrypt for free, automatically-renewed certificates.

### Using Certbot

```bash
# Install certbot
sudo apt-get install certbot  # Debian/Ubuntu
sudo yum install certbot      # RHEL/CentOS

# Obtain certificate (standalone mode)
sudo certbot certonly --standalone -d relay.example.com

# Certificate files will be created at:
# /etc/letsencrypt/live/relay.example.com/fullchain.pem
# /etc/letsencrypt/live/relay.example.com/privkey.pem
```

Note: The standalone method requires port 80 to be available temporarily during certificate issuance.

## Option 3: Corporate CA Certificates

If you have an internal Certificate Authority, request a certificate for your relay service hostname and obtain:
- The server certificate (PEM format)
- The private key (PEM format)

Consult your organisation's PKI documentation for the certificate request process.

## Configure Relay with TLS

### File Permissions

Secure your certificate files:

```bash
# Set restrictive permissions on the private key
chmod 600 relay-key.pem
chmod 644 relay-cert.pem

# If running relay as a specific user
chown relay:relay relay-key.pem relay-cert.pem
```

### Update Configuration File

Edit your relay configuration file (e.g., `config.yml`):

```yaml
listeners:
  - name: "user-activity"
    listen_addr: ":9015"
    log_type: "user-activity"
    output_dir: "./zpa-logs"
    file_prefix: "zpa-user-activity"
    tls:
      cert_file: "/path/to/relay-cert.pem"
      key_file: "/path/to/relay-key.pem"
    splunk:
      source_type: "zpa:user:activity"
```

Both `cert_file` and `key_file` must be specified together. Use absolute paths to avoid issues with working directory changes.

### Start Relay

```bash
./relay --config config.yml
```

You should see:
```
listening TLS on :9015
```

If you see errors, check the validation section below.

## Test the TLS Connection

### Using OpenSSL Client

```bash
# Test TLS connection
openssl s_client -connect localhost:9015 -showcerts

# Test with specific TLS version
openssl s_client -connect localhost:9015 -tls1_2
```

Successful output includes:
```
SSL handshake has read ... bytes
...
Verify return code: 0 (ok)
```

### Using netcat with OpenSSL

```bash
# Send a test JSON line
echo '{"test": "message"}' | openssl s_client -connect localhost:9015 -quiet
```

### Test with Self-Signed Certificate

When using self-signed certificates, you may see verification warnings. This is expected. You can bypass verification for testing:

```bash
openssl s_client -connect localhost:9015 -CAfile relay-cert.pem
```

## Configure Zscaler ZPA LSS

In the Zscaler ZPA admin console:

1. Navigate to Administration > Log Streaming Service
2. Configure your log receiver:
   - **Protocol**: TLS (or TCP/TLS depending on UI)
   - **Host**: Your relay server hostname or IP
   - **Port**: Your relay listener port (e.g., 9015)
3. If using self-signed certificates, you may need to:
   - Disable certificate verification (not recommended for production)
   - Upload your CA certificate to ZPA

Consult Zscaler documentation for specific ZPA LSS TLS configuration requirements.

## Common Issues

### Certificate Validation Errors

```
Error: listener user-activity: failed to load TLS certificate: tls: failed to find any PEM data
```

**Solutions:**
- Verify files are in PEM format (text files starting with `-----BEGIN CERTIFICATE-----`)
- Check file paths are correct and absolute
- Ensure files are readable by the relay process user

### Certificate/Key Mismatch

```
Error: listener user-activity: failed to load TLS certificate: tls: private key does not match public key
```

**Solutions:**
- Verify the certificate and key were generated together
- Use the verification commands shown above to check they match
- Regenerate certificate and key if necessary

### Permission Denied

```
Error: listener user-activity: TLS cert file not accessible: open /path/to/cert.pem: permission denied
```

**Solutions:**
- Check file permissions with `ls -l`
- Ensure the relay process user can read the files
- Verify SELinux/AppArmor policies if applicable

### Port Already in Use

```
Error: listener user-activity: cannot bind to listen address: address already in use
```

**Solutions:**
- Check if another service is using the port: `netstat -tuln | grep 9015`
- Change the listen port in your configuration
- Stop the conflicting service

## Security Best Practices

### Certificate Management

1. **Use Strong Key Sizes**: Minimum 2048-bit RSA, prefer 4096-bit for long-lived certificates
2. **Set Appropriate Validity Periods**:
   - Internal certificates: 1-2 years
   - Public certificates: 90 days (Let's Encrypt default)
3. **Rotate Certificates Before Expiration**: Set calendar reminders
4. **Secure Private Keys**:
   - Never commit keys to version control
   - Use file permissions 600 (owner read/write only)
   - Store in encrypted volumes when possible

### TLS Configuration

The relay enforces TLS 1.2 as the minimum version. This is configured in the server code and cannot be changed without modifying the source.

Supported cipher suites are determined by the Go standard library's default TLS configuration, which follows security best practices.

### Monitoring Certificate Expiry

Check certificate expiration date:

```bash
openssl x509 -enddate -noout -in relay-cert.pem
```

Set up monitoring to alert before expiration (recommend 30 days before).

## Certificate Renewal

### Self-Signed Certificates

1. Generate new certificate using the commands above
2. Update configuration file with new paths (or overwrite existing files)
3. Restart relay service

```bash
sudo systemctl restart relay
```

### Let's Encrypt Certificates

Certbot handles renewal automatically. To manually renew:

```bash
sudo certbot renew
sudo systemctl restart relay
```

### Corporate CA Certificates

Follow your organisation's certificate renewal process, then restart the relay service.

## Testing Checklist

Before deploying to production:

- [ ] Certificate and key files exist and are readable
- [ ] Certificate has not expired
- [ ] Certificate and key match (verified with openssl)
- [ ] Relay starts without TLS errors
- [ ] OpenSSL client can connect successfully
- [ ] Test JSON message can be sent and received
- [ ] ZPA LSS can connect and send logs
- [ ] Logs appear in local storage files
- [ ] Logs forward to Splunk HEC (if configured)

## Further Reading

- [OpenSSL Documentation](https://www.openssl.org/docs/)
- [Let's Encrypt Documentation](https://letsencrypt.org/docs/)
- [Zscaler ZPA LSS Configuration Guide](https://help.zscaler.com/)
- [Mozilla SSL Configuration Generator](https://ssl-config.mozilla.org/)

## Troubleshooting

If you continue to experience issues:

1. Check relay logs for detailed error messages
2. Verify certificate details with OpenSSL commands
3. Test connectivity without TLS first (temporarily remove TLS config)
4. Consult the main troubleshooting section in the README
5. Open an issue on GitHub with error messages and configuration details
