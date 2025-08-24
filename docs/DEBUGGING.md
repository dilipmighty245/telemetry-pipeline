# Debugging and Troubleshooting Guide

## Overview

This guide provides comprehensive troubleshooting steps, debugging techniques, and solutions for common issues in the Telemetry Pipeline. Use this guide to quickly identify and resolve problems in both local and production environments.

## 🚨 Quick Diagnostics

### Health Check Commands

```bash
# Local Environment
make scale-status                    # Check local scaling status
curl http://localhost:8080/health    # Test API Gateway health
etcdctl --endpoints=localhost:2379 endpoint health  # Check etcd

# Kubernetes Environment
make k8s-status-nexus               # Check K8s deployment status
kubectl get pods -n telemetry-system  # Check pod status
kubectl get events -n telemetry-system --sort-by='.lastTimestamp'  # Recent events
```

### Component Status Overview

```bash
# Check all components at once
./scripts/health-check.sh           # Comprehensive health check (if exists)

# Manual checks
ps aux | grep nexus                 # Local processes
kubectl top pods -n telemetry-system  # K8s resource usage
```

## 🔍 Component-Specific Debugging

### 🔄 Nexus Streamer Issues

#### Common Problems

**Problem**: Streamer not processing CSV data
```bash
# Check if CSV file exists and is readable
ls -la dcgm_metrics_20250718_134233.csv
head -5 dcgm_metrics_20250718_134233.csv

# Check streamer logs
tail -f logs/streamer-*.log
# Or in Kubernetes:
kubectl logs -f deployment/telemetry-pipeline-nexus-streamer -n telemetry-system
```

**Problem**: High memory usage in streamer
```bash
# Check batch size configuration
env | grep BATCH_SIZE

# Monitor memory usage
top -p $(pgrep nexus-streamer)
# Or in Kubernetes:
kubectl top pods -l app.kubernetes.io/component=nexus-streamer -n telemetry-system
```

**Problem**: Streamer can't connect to etcd
```bash
# Test etcd connectivity
etcdctl --endpoints=localhost:2379 endpoint health
etcdctl --endpoints=localhost:2379 put test-key test-value
etcdctl --endpoints=localhost:2379 get test-key
etcdctl --endpoints=localhost:2379 del test-key

# Check network connectivity
telnet localhost 2379
# Or in Kubernetes:
kubectl exec -it deployment/telemetry-pipeline-nexus-streamer -n telemetry-system -- nc -zv telemetry-pipeline-etcd 2379
```

#### Debug Commands

```bash
# Enable debug logging
LOG_LEVEL=debug make run-nexus-streamer

# Check message queue status
etcdctl --endpoints=localhost:2379 get --prefix "/messagequeue/" --keys-only

# Monitor streaming rate
watch -n 1 'etcdctl --endpoints=localhost:2379 get --prefix "/messagequeue/" --keys-only | wc -l'
```

### ⚙️ Nexus Collector Issues

#### Common Problems

**Problem**: Collector not processing messages
```bash
# Check message queue has data
etcdctl --endpoints=localhost:2379 get --prefix "/messagequeue/" --keys-only | head -10

# Check collector logs for errors
tail -f logs/collector-*.log | grep -i error
# Or in Kubernetes:
kubectl logs -f deployment/telemetry-pipeline-nexus-collector -n telemetry-system | grep -i error
```

**Problem**: Data not being stored properly
```bash
# Check stored telemetry data
etcdctl --endpoints=localhost:2379 get --prefix "/telemetry/clusters/local-cluster/" --keys-only | head -20

# Verify GPU registration
etcdctl --endpoints=localhost:2379 get --prefix "/telemetry/clusters/local-cluster/hosts/" --keys-only | grep "/gpus/" | head -10

# Check data format
etcdctl --endpoints=localhost:2379 get "/telemetry/clusters/local-cluster/hosts/mtv5-dgx1-hgpu-032/gpus/0"
```

**Problem**: Collector processing slowly
```bash
# Check processing interval
env | grep PROCESSING_INTERVAL

# Monitor processing rate
watch -n 5 'etcdctl --endpoints=localhost:2379 get --prefix "/telemetry/" --keys-only | wc -l'

# Check for deadlocks or blocking operations
pstack $(pgrep nexus-collector)  # Linux
# Or check CPU usage patterns
top -p $(pgrep nexus-collector)
```

#### Debug Commands

```bash
# Enable debug logging
LOG_LEVEL=debug make run-nexus-collector

# Monitor message consumption
etcdctl --endpoints=localhost:2379 watch --prefix "/messagequeue/" --print-value-only

# Check collector performance metrics
curl http://localhost:9091/metrics  # If metrics endpoint enabled
```

### 🌐 Nexus Gateway Issues

#### Common Problems

**Problem**: API returning empty results
```bash
# Test basic connectivity
curl -v http://localhost:8080/health

# Check if GPUs are registered
curl -s http://localhost:8080/api/v1/gpus | jq '.count'

# Verify etcd has data
etcdctl --endpoints=localhost:2379 get --prefix "/telemetry/clusters/local-cluster/hosts/" --keys-only | wc -l

# Check specific GPU data
curl -s http://localhost:8080/api/v1/gpus | jq '.data[0]'
```

**Problem**: Slow API responses
```bash
# Test response times
time curl -s http://localhost:8080/api/v1/gpus > /dev/null

# Check etcd query performance
time etcdctl --endpoints=localhost:2379 get --prefix "/telemetry/clusters/local-cluster/" --keys-only > /dev/null

# Monitor gateway logs for slow queries
tail -f logs/gateway-*.log | grep -E "(slow|timeout|latency)"
```

**Problem**: WebSocket connections failing
```bash
# Test WebSocket connection
wscat -c ws://localhost:8080/ws

# Check gateway WebSocket support
curl -H "Connection: Upgrade" -H "Upgrade: websocket" http://localhost:8080/ws

# Verify WebSocket configuration
env | grep ENABLE_WEBSOCKET
```

#### Debug Commands

```bash
# Enable debug logging
LOG_LEVEL=debug ENABLE_GRAPHQL=true ENABLE_WEBSOCKET=true make run-nexus-gateway

# Test all API endpoints
curl -s http://localhost:8080/api/v1/gpus | jq .
curl -s http://localhost:8080/api/v1/gpus/GPU-uuid-here/telemetry | jq .

# Test GraphQL endpoint
curl -X POST http://localhost:8080/graphql \
  -H "Content-Type: application/json" \
  -d '{"query": "{ gpus { id uuid hostname } }"}'
```

### 🗄️ etcd Issues

#### Common Problems

**Problem**: etcd not starting
```bash
# Check etcd service status
systemctl status etcd  # If using systemd
docker ps | grep etcd  # If using Docker

# Check etcd logs
journalctl -u etcd -f  # systemd
docker logs etcd-container  # Docker

# Verify etcd data directory
ls -la /var/lib/etcd/  # Default data directory
```

**Problem**: etcd cluster issues
```bash
# Check cluster health
etcdctl --endpoints=localhost:2379 endpoint health --cluster

# Check cluster members
etcdctl --endpoints=localhost:2379 member list

# Check cluster status
etcdctl --endpoints=localhost:2379 endpoint status --cluster --write-out=table
```

**Problem**: etcd performance issues
```bash
# Check etcd metrics
curl http://localhost:2379/metrics | grep etcd

# Monitor disk I/O
iostat -x 1  # Linux
# Check disk space
df -h /var/lib/etcd/

# Check memory usage
free -h
top -p $(pgrep etcd)
```

## 🐛 Common Error Scenarios

### Error: "Connection refused"

**Symptoms**: Components can't connect to etcd
```bash
# Check if etcd is running
ps aux | grep etcd
netstat -tulpn | grep :2379

# Test connection
telnet localhost 2379

# Check firewall
sudo ufw status  # Ubuntu
sudo firewall-cmd --list-ports  # CentOS/RHEL
```

**Solutions**:
1. Start etcd: `make setup-etcd`
2. Check firewall settings
3. Verify etcd configuration

### Error: "No space left on device"

**Symptoms**: etcd or components failing due to disk space
```bash
# Check disk usage
df -h
du -sh /var/lib/etcd/
du -sh logs/

# Check inode usage
df -i
```

**Solutions**:
1. Clean up old logs: `find logs/ -name "*.log" -mtime +7 -delete`
2. Compact etcd: `etcdctl --endpoints=localhost:2379 compact $(etcdctl --endpoints=localhost:2379 endpoint status --write-out="json" | jq -r '.[] | .Status.header.revision')`
3. Increase disk space

### Error: "Too many open files"

**Symptoms**: Components failing with file descriptor limits
```bash
# Check current limits
ulimit -n
cat /proc/$(pgrep nexus-gateway)/limits | grep "Max open files"

# Check actual usage
lsof -p $(pgrep nexus-gateway) | wc -l
```

**Solutions**:
1. Increase limits: `ulimit -n 65536`
2. Update systemd limits: Edit `/etc/systemd/system.conf`
3. Check for resource leaks

### Error: "Context deadline exceeded"

**Symptoms**: Timeouts in etcd operations
```bash
# Check etcd response times
time etcdctl --endpoints=localhost:2379 get --prefix "/telemetry/" --keys-only

# Monitor etcd performance
etcdctl --endpoints=localhost:2379 endpoint status --write-out=table
```

**Solutions**:
1. Increase timeout values in configuration
2. Optimize etcd performance
3. Check network latency

## 🔧 Performance Debugging

### Memory Issues

```bash
# Check memory usage by component
ps aux --sort=-%mem | grep nexus

# Monitor memory over time
while true; do
  ps -o pid,comm,%mem --sort=-%mem | grep nexus
  sleep 5
done

# Check for memory leaks
valgrind --tool=memcheck --leak-check=full ./bin/nexus-gateway  # Development only
```

### CPU Issues

```bash
# Check CPU usage
top -p $(pgrep nexus)

# Profile CPU usage
perf record -p $(pgrep nexus-gateway) -g -- sleep 30
perf report

# Check for CPU-intensive operations
strace -p $(pgrep nexus-collector) -c -S time
```

### Network Issues

```bash
# Check network connections
netstat -tulpn | grep nexus
ss -tulpn | grep nexus

# Monitor network traffic
iftop -i eth0
nethogs

# Check for network errors
ethtool -S eth0 | grep error
```

## 📊 Monitoring and Metrics

### Local Monitoring

```bash
# Component resource usage
htop -p $(pgrep nexus | tr '\n' ',' | sed 's/,$//')

# Network monitoring
iftop -P -i eth0

# Disk I/O monitoring
iotop -p $(pgrep nexus | tr '\n' ',' | sed 's/,$//')

# Log monitoring
multitail logs/streamer-*.log logs/collector-*.log logs/gateway-*.log
```

### Kubernetes Monitoring

```bash
# Pod resource usage
kubectl top pods -n telemetry-system

# Node resource usage
kubectl top nodes

# Pod events
kubectl get events -n telemetry-system --sort-by='.lastTimestamp' | tail -20

# Detailed pod information
kubectl describe pod -l app.kubernetes.io/name=telemetry-pipeline -n telemetry-system
```

## 🔄 Recovery Procedures

### Component Recovery

```bash
# Restart individual components locally
make scale-stop
INSTANCES=2 make scale-local

# Restart in Kubernetes
kubectl rollout restart deployment/telemetry-pipeline-nexus-streamer -n telemetry-system
kubectl rollout restart deployment/telemetry-pipeline-nexus-collector -n telemetry-system
kubectl rollout restart deployment/telemetry-pipeline-nexus-gateway -n telemetry-system
```

### Data Recovery

```bash
# Backup etcd data
etcdctl --endpoints=localhost:2379 snapshot save backup.db

# Restore etcd data (if needed)
etcdctl snapshot restore backup.db --data-dir=/var/lib/etcd-restore/

# Clean up corrupted data
etcdctl --endpoints=localhost:2379 del --prefix "/telemetry/corrupted-key"
```

### Complete System Recovery

```bash
# Local environment
make scale-stop
make setup-etcd
INSTANCES=2 make scale-local

# Kubernetes environment
make k8s-undeploy-nexus
make k8s-deploy-nexus
```

## 📋 Debug Checklists

### Pre-Deployment Checklist

- [ ] etcd is running and accessible
- [ ] CSV data file exists and is readable
- [ ] Network ports are available (8080+, 2379)
- [ ] Sufficient disk space (>1GB free)
- [ ] Sufficient memory (>2GB available)
- [ ] All binaries are built and executable

### Runtime Checklist

- [ ] All components are running (check `make scale-status`)
- [ ] etcd has data (check keys with `etcdctl`)
- [ ] API Gateway responds to health checks
- [ ] GPUs are registered (check `/api/v1/gpus`)
- [ ] Telemetry data is being processed
- [ ] No error messages in logs

### Performance Checklist

- [ ] CPU usage < 80% per component
- [ ] Memory usage < 80% per component
- [ ] Disk usage < 90%
- [ ] Network latency < 10ms to etcd
- [ ] API response time < 100ms
- [ ] Processing rate matches ingestion rate

## 🆘 Getting Help

### Log Analysis

```bash
# Collect all logs for analysis
mkdir -p debug-logs/$(date +%Y%m%d-%H%M%S)
cp logs/*.log debug-logs/$(date +%Y%m%d-%H%M%S)/

# In Kubernetes
kubectl logs -l app.kubernetes.io/name=telemetry-pipeline -n telemetry-system --all-containers=true > debug-logs/k8s-logs.txt
```

### System Information

```bash
# Collect system information
uname -a > system-info.txt
free -h >> system-info.txt
df -h >> system-info.txt
ps aux | grep nexus >> system-info.txt
netstat -tulpn | grep -E "(2379|8080)" >> system-info.txt
```

### Configuration Dump

```bash
# Export current configuration
env | grep -E "(CLUSTER_ID|ETCD|LOG_LEVEL|BATCH_SIZE)" > config-dump.txt
make scale-status >> config-dump.txt
etcdctl --endpoints=localhost:2379 get --prefix "/telemetry/" --keys-only | head -50 >> config-dump.txt
```

When reporting issues, please include:
1. Component logs
2. System information
3. Configuration dump
4. Steps to reproduce
5. Expected vs actual behavior
