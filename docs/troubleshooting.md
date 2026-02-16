# Troubleshooting Guide

Common issues and solutions when using Qwencoder Proxy.

---

## Table of Contents

- [Getting Started Issues](#getting-started-issues)
- [Authentication Issues](#authentication-issues)
- [API Request Issues](#api-request-issues)
- [Provider-Specific Issues](#provider-specific-issues)
- [Performance Issues](#performance-issues)
- [Streaming Issues](#streaming-issues)
- [Network Issues](#network-issues)
- [Debug Mode](#debug-mode)

---

## Getting Started Issues

### Server Won't Start

**Symptom:** The proxy server fails to start or exits immediately.

**Possible Causes:**
1. Port already in use
2. Configuration file errors
3. Missing dependencies

**Solutions:**

1. **Check port availability:**
   ```bash
   # Check if port 8143 is in use (Linux/Mac)
   lsof -i :8143
   
   # Check if port 8143 is in use (Windows)
   netstat -ano | findstr :8143
   ```

2. **Change port in configuration:**
   ```yaml
   # config.yaml
   port: "8144"
   ```

3. **Validate configuration file:**
   ```bash
   # Check for YAML syntax errors
   python -c "import yaml; yaml.safe_load(open('config.yaml'))"
   ```

4. **Check logs:**
   ```bash
   # Run with verbose logging
   ./qwencoder-proxy --verbose
   ```

### Cannot Connect to Proxy

**Symptom:** Connection refused or timeout when accessing `http://localhost:8143`

**Possible Causes:**
1. Server not running
2. Firewall blocking connection
3. Wrong port

**Solutions:**

1. **Verify server is running:**
   ```bash
   # Check process
   ps aux | grep qwencoder-proxy  # Linux/Mac
   tasklist | findstr qwencoder-proxy  # Windows
   ```

2. **Check firewall settings:**
   ```bash
   # Allow port through firewall (Linux)
   sudo ufw allow 8143
   
   # Allow port through firewall (Windows)
   netsh advfirewall firewall add rule name="Qwencoder Proxy" dir=in action=allow protocol=TCP localport=8143
   ```

3. **Test connection:**
   ```bash
   curl http://localhost:8143/v1/models
   ```

---

## Authentication Issues

### 401 Unauthorized Errors

**Symptom:** API requests return 401 Unauthorized.

**Possible Causes:**
1. Invalid API key
2. Expired token
3. Missing credentials

**Solutions:**

1. **Verify credentials in dashboard:**
   - Open `http://localhost:8080`
   - Check provider authentication status
   - Re-authenticate if needed

2. **Check token expiry:**
   ```bash
   curl -X GET http://localhost:8080/api/qwen/tokens
   ```

3. **Re-authenticate:**
   ```bash
   # For OAuth2 providers
   curl -X POST http://localhost:8080/api/qwen/device-code
   
   # For API key providers
   curl -X POST http://localhost:8080/api/gemini/credentials \
     -H "Content-Type: application/json" \
     -d '{"api_key": "your-api-key"}'
   ```

### OAuth2 Flow Fails

**Symptom:** Device code flow doesn't complete or authorization fails.

**Possible Causes:**
1. Expired device code
2. Incorrect user code
3. Network issues

**Solutions:**

1. **Start new device code flow:**
   ```bash
   curl -X POST http://localhost:8080/api/qwen/device-code
   ```

2. **Enter user code correctly:**
   - User codes are case-sensitive
   - Format is typically `XXXX-XXXX`

3. **Check verification URL:**
   - Ensure you're visiting the correct URL
   - Some providers may use different URLs

4. **Check network connectivity:**
   ```bash
   # Test connectivity to provider
   curl -I https://qwen.com
   ```

### Token Refresh Fails

**Symptom:** Tokens fail to refresh automatically.

**Possible Causes:**
1. Refresh token expired
2. Refresh token revoked
3. Network issues

**Solutions:**

1. **Re-authenticate:**
   - Use the web dashboard to re-authenticate
   - This generates new refresh tokens

2. **Check token status:**
   ```bash
   curl -X GET http://localhost:8080/api/qwen/tokens
   ```

3. **Manually refresh:**
   ```bash
   curl -X POST http://localhost:8080/api/qwen/refresh
   ```

---

## API Request Issues

### 400 Bad Request

**Symptom:** API requests return 400 Bad Request.

**Possible Causes:**
1. Invalid request format
2. Missing required parameters
3. Invalid model name

**Solutions:**

1. **Validate request format:**
   ```json
   {
     "model": "qwen-max",
     "messages": [
       {"role": "user", "content": "Hello!"}
     ]
   }
   ```

2. **Check model name:**
   ```bash
   # List available models
   curl http://localhost:8143/v1/models
   ```

3. **Verify JSON syntax:**
   ```bash
   # Validate JSON
   echo '{"model": "qwen-max", "messages": [...]}' | jq .
   ```

### 404 Not Found

**Symptom:** API requests return 404 Not Found.

**Possible Causes:**
1. Invalid endpoint
2. Model not supported
3. Provider not configured

**Solutions:**

1. **Check endpoint:**
   ```bash
   # Correct endpoints
   GET /v1/models
   POST /v1/chat/completions
   ```

2. **Verify model is available:**
   ```bash
   curl http://localhost:8143/v1/models | jq '.data[].id'
   ```

3. **Check provider configuration:**
   - Open dashboard at `http://localhost:8080`
   - Verify provider is authenticated
   - Check provider health status

### 500 Internal Server Error

**Symptom:** API requests return 500 Internal Server Error.

**Possible Causes:**
1. Provider API issues
2. Protocol conversion errors
3. Network issues

**Solutions:**

1. **Check proxy logs:**
   ```bash
   # View logs
   tail -f ~/.qwencoder-proxy/logs/proxy.log
   ```

2. **Test provider directly:**
   - Try calling the provider API directly
   - Verify provider credentials are valid

3. **Check network connectivity:**
   ```bash
   # Test connection to provider
   curl -I https://api.qwen.com
   ```

4. **Try alternative provider:**
   ```bash
   # Use provider-specific endpoint
   curl -X POST http://localhost:8143/qwen/v1/chat/completions \
     -H "Content-Type: application/json" \
     -d '{"model": "qwen-max", "messages": [...]}'
   ```

---

## Provider-Specific Issues

### Qwen Issues

#### Connection Timeout

**Symptom:** Requests to Qwen timeout.

**Solutions:**

1. **Increase timeout:**
   ```yaml
   # config.yaml
   providers:
     qwen:
       timeout: 60
   ```

2. **Check proxy settings:**
   ```bash
   curl -X GET http://localhost:8080/api/qwen/tokens
   ```

3. **Test connectivity:**
   ```bash
   curl -I https://dashscope.aliyuncs.com
   ```

### Gemini Issues

#### Invalid API Key

**Symptom:** 401 Unauthorized with Gemini.

**Solutions:**

1. **Verify API key:**
   - Check Google Cloud Console
   - Ensure API key has correct permissions

2. **Re-enter API key:**
   ```bash
   curl -X POST http://localhost:8080/api/gemini/credentials \
     -H "Content-Type: application/json" \
     -d '{"api_key": "your-api-key"}'
   ```

### Kiro (Claude) Issues

#### Rate Limit Exceeded

**Symptom:** 429 Too Many Requests.

**Solutions:**

1. **Wait and retry:**
   - Implement exponential backoff
   - Respect rate limits

2. **Use multiple tokens:**
   - Configure multiple Anthropic API keys
   - The proxy will load balance

3. **Reduce request frequency:**
   - Cache responses when possible
   - Batch requests

### iFlow Issues

#### Model Not Available

**Symptom:** Model not found error.

**Solutions:**

1. **Check available models:**
   ```bash
   curl http://localhost:8143/iflow/v1/models
   ```

2. **Verify model name:**
   - Check iFlow documentation
   - Use correct model identifier

---

## Performance Issues

### Slow Response Times

**Symptom:** API requests take too long to complete.

**Possible Causes:**
1. Network latency
2. Provider performance
3. Large requests

**Solutions:**

1. **Measure response time:**
   ```bash
   time curl -X POST http://localhost:8143/v1/chat/completions \
     -H "Content-Type: application/json" \
     -d '{"model": "qwen-max", "messages": [...]}'
   ```

2. **Use streaming for long responses:**
   ```json
   {
     "model": "qwen-max",
     "messages": [...],
     "stream": true
   }
   ```

3. **Optimize request size:**
   - Reduce context length
   - Use appropriate `max_tokens`
   - Minimize system prompt

4. **Try faster models:**
   ```json
   {
     "model": "gemini-1.5-flash",  // Faster than gemini-1.5-pro
     "messages": [...]
   }
   ```

### High Memory Usage

**Symptom:** Proxy process consumes excessive memory.

**Possible Causes:**
1. Large streaming responses
2. Many concurrent requests
3. Memory leaks

**Solutions:**

1. **Limit concurrent requests:**
   ```yaml
   # config.yaml
   max_concurrent_requests: 10
   ```

2. **Monitor memory usage:**
   ```bash
   # Check process memory
   ps aux | grep qwencoder-proxy
   
   # Or use htop/htop
   htop
   ```

3. **Restart proxy regularly:**
   - Set up automatic restart
   - Use process manager (systemd, supervisord)

---

## Streaming Issues

### Stream Stops Prematurely

**Symptom:** Streaming response stops before completion.

**Possible Causes:**
1. Network interruption
2. Provider error
3. Timeout

**Solutions:**

1. **Implement retry logic:**
   ```python
   def stream_with_retry(prompt, max_retries=3):
       for attempt in range(max_retries):
           try:
               return stream_completion(prompt)
           except Exception as e:
               if attempt == max_retries - 1:
                   raise
               time.sleep(2 ** attempt)  # Exponential backoff
   ```

2. **Increase timeout:**
   ```yaml
   # config.yaml
   stream_timeout: 300  # 5 minutes
   ```

3. **Check network stability:**
   - Ensure stable connection
   - Consider using keep-alive

### Stream Chunk Format Errors

**Symptom:** Invalid JSON in stream chunks.

**Possible Causes:**
1. Protocol conversion issues
2. Provider API changes
3. Network corruption

**Solutions:**

1. **Implement robust parsing:**
   ```python
   import json
   
   for line in response.iter_lines():
       if line:
           line = line.decode('utf-8')
           if line.startswith('data: '):
               data = line[6:]
               if data == '[DONE]':
                   break
               try:
                   chunk = json.loads(data)
                   # Process chunk
               except json.JSONDecodeError:
                   # Skip invalid chunks
                   continue
   ```

2. **Enable debug logging:**
   ```bash
   export QWENCODER_DEBUG=true
   ```

3. **Report issue:**
   - Check for proxy updates
   - Report bug with logs

---

## Network Issues

### Connection Refused

**Symptom:** Cannot connect to proxy server.

**Solutions:**

1. **Verify server is running:**
   ```bash
   ps aux | grep qwencoder-proxy
   ```

2. **Check firewall:**
   ```bash
   # Linux
   sudo ufw status
   
   # Windows
   netsh advfirewall show allprofiles
   ```

3. **Test localhost:**
   ```bash
   curl http://localhost:8143/v1/models
   ```

### Proxy Connection Issues

**Symptom:** Cannot connect to provider through proxy.

**Solutions:**

1. **Verify proxy URL:**
   ```bash
   curl -X GET http://localhost:8080/api/qwen/tokens
   ```

2. **Test proxy directly:**
   ```bash
   curl -x http://proxy.example.com:8080 https://api.qwen.com
   ```

3. **Check proxy authentication:**
   - Verify proxy username/password
   - Update credentials in dashboard

### SSL/TLS Errors

**Symptom:** SSL certificate errors.

**Solutions:**

1. **Update CA certificates:**
   ```bash
   # Linux
   sudo update-ca-certificates
   
   # Mac
   brew install ca-certificates
   ```

2. **Disable SSL verification (not recommended for production):**
   ```yaml
   # config.yaml
   ssl_verify: false
   ```

3. **Use custom CA bundle:**
   ```yaml
   # config.yaml
   ca_bundle: "/path/to/ca-bundle.crt"
   ```

---

## Debug Mode

### Enable Debug Logging

Enable verbose logging to troubleshoot issues:

```bash
# Environment variable
export QWENCODER_DEBUG=true

# Or in config file
debug: true
```

### View Logs

Logs are stored in `~/.qwencoder-proxy/logs/`:

```bash
# View proxy logs
tail -f ~/.qwencoder-proxy/logs/proxy.log

# View authentication logs
tail -f ~/.qwencoder-proxy/logs/auth.log

# View error logs
tail -f ~/.qwencoder-proxy/logs/error.log
```

### Enable Request/Response Logging

Log all API requests and responses:

```yaml
# config.yaml
logging:
  level: debug
  log_requests: true
  log_responses: true
```

### Test Endpoints

Use test endpoints to verify configuration:

```bash
# Health check
curl http://localhost:8143/health

# List providers
curl http://localhost:8143/providers

# Provider status
curl http://localhost:8143/providers/qwen/status
```

---

## Getting Help

If you're still experiencing issues:

1. **Check the logs:**
   ```bash
   tail -f ~/.qwencoder-proxy/logs/proxy.log
   ```

2. **Search existing issues:**
   - Check [GitHub Issues](https://github.com/sunbankio/qwencoder-proxy/issues)
   - Search for similar problems

3. **Create a new issue:**
   - Include error messages
   - Attach relevant logs
   - Describe your configuration
   - Specify your environment (OS, Go version, etc.)

4. **Community support:**
   - Join the community chat
   - Ask questions in discussions

---

## Common Error Messages

| Error | Cause | Solution |
|-------|-------|----------|
| `connection refused` | Server not running | Start the proxy server |
| `401 unauthorized` | Invalid credentials | Re-authenticate or update API key |
| `404 not found` | Invalid endpoint or model | Check endpoint URL and model name |
| `429 too many requests` | Rate limit exceeded | Wait and retry, or use multiple tokens |
| `500 internal server error` | Provider error | Check logs and provider status |
| `timeout` | Network or provider issue | Increase timeout or check network |
| `invalid model` | Model not supported | Check available models |
| `missing parameter` | Incomplete request | Add required parameters |
| `json decode error` | Invalid JSON format | Validate request JSON |
| `stream error` | Streaming interrupted | Implement retry logic |

---

## Preventive Measures

### Regular Maintenance

1. **Update proxy regularly:**
   ```bash
   git pull origin main
   go build -o qwencoder-proxy cmd/qwencoder-proxy/main.go
   ```

2. **Monitor logs:**
   - Set up log rotation
   - Monitor error rates
   - Set up alerts

3. **Backup credentials:**
   - Regularly backup `~/.qwencoder-proxy/`
   - Store backup securely

### Monitoring

1. **Health checks:**
   ```bash
   # Add to cron or monitoring system
   curl http://localhost:8143/health
   ```

2. **Token health:**
   ```bash
   # Check token health regularly
   curl -X GET http://localhost:8080/api/qwen/tokens
   ```

3. **Performance metrics:**
   - Monitor response times
   - Track error rates
   - Measure resource usage

### Configuration Best Practices

1. **Use environment variables for secrets:**
   ```bash
   export QWEN_API_KEY="your-api-key"
   export GEMINI_API_KEY="your-api-key"
   ```

2. **Separate configs for different environments:**
   - `config.dev.yaml`
   - `config.prod.yaml`
   - `config.test.yaml`

3. **Document your configuration:**
   - Keep notes on custom settings
   - Document provider-specific configurations
   - Maintain changelog of changes
