# Agent Examples

This document provides comprehensive examples of GAPI agents for various use cases.

## Service Agents

Service agents run continuously until stopped.

### Basic Service

```python
# agents/hello.py.service
ENABLED = True
TYPE = "service"

import time

def start():
    print("Hello from service agent!")
    while True:
        print("Still running...")
        time.sleep(60)
```

### Service with Cleanup

```python
# agents/worker.py.service
ENABLED = True
TYPE = "service"

import signal
import sys

running = True

def start():
    global running
    
    # Setup signal handlers
    signal.signal(signal.SIGTERM, lambda s, f: stop())
    
    print("Worker started")
    while running:
        # Do work
        time.sleep(1)

def stop():
    global running
    print("Worker stopping...")
    running = False
    # Cleanup resources
```

### Service with Dependencies

```python
# agents/api.py.service
# ENABLED = True
# TYPE = service
# DEPENDENCIES = database, cache

def start():
    # Database and cache are guaranteed to be running
    print("API server starting...")
    # Start HTTP server
```

## Timer Agents

Timer agents execute on a schedule.

### Systemd-Style Intervals

```python
# agents/backup.py.timer
ENABLED = True
TYPE = "timer"
SCHEDULE = "OnUnitActiveSec=5m"

def start():
    print("Running backup...")
    # Backup logic here
    # Agent exits when complete
```

### Boot/Startup Timers

```python
# agents/init-check.py.timer
ENABLED = True
TYPE = "timer"
SCHEDULE = "OnBootSec=30s"

def start():
    print("Running post-boot check...")
    # Runs 30 seconds after system boot
```

```python
# agents/warmup.py.timer
ENABLED = True
TYPE = "timer"
SCHEDULE = "OnStartupSec=10s"

def start():
    print("Warming up caches...")
    # Runs 10 seconds after supervisor starts
```

### Cron-Style Schedules

```python
# agents/daily-report.py.timer
ENABLED = True
TYPE = "timer"
SCHEDULE = "0 9 * * 1-5"

def start():
    print("Generating daily report...")
    # Runs at 9 AM on weekdays
```

```python
# agents/hourly-sync.py.timer
ENABLED = True
TYPE = "timer"
SCHEDULE = "0 * * * *"

def start():
    print("Syncing data...")
    # Runs at the top of every hour
```

### Named Schedules

```python
# agents/cleanup.py.timer
ENABLED = True
TYPE = "timer"
SCHEDULE = "@daily"

def start():
    print("Running daily cleanup...")
    # Runs at midnight every day
```

### Raw Duration

```python
# agents/heartbeat.py.timer
ENABLED = True
TYPE = "timer"
SCHEDULE = "30s"

def start():
    print("Heartbeat")
    # Runs every 30 seconds
```

## Socket-Activated Agents

Socket-activated agents start on-demand when a connection is received.

### TCP Socket

```python
# agents/echo-server.py.socket
# ENABLED = True
# TYPE = socket
# LISTEN_STREAM = 0.0.0.0:8080

import os
import socket

def start():
    fd = int(os.environ.get("LISTEN_FDS", "0"))
    if fd == 0:
        print("No socket provided")
        return
    
    # Get the listening socket (fd 3)
    sock = socket.fromfd(fd + 3, socket.AF_INET, socket.SOCK_STREAM)
    
    while True:
        conn, addr = sock.accept()
        print(f"Connection from {addr}")
        data = conn.recv(1024)
        conn.sendall(data)  # Echo back
        conn.close()
```

### UDP Socket

```python
# agents/dns-responder.py.socket
# ENABLED = True
# TYPE = socket
# LISTEN_DATAGRAM = 0.0.0.0:5353

import os
import socket

def start():
    fd = int(os.environ.get("LISTEN_FDS", "0"))
    if fd == 0:
        return
    
    sock = socket.fromfd(fd + 3, socket.AF_INET, socket.SOCK_DGRAM)
    
    while True:
        data, addr = sock.recvfrom(1024)
        print(f"Received from {addr}: {data}")
        # Process and respond
```

### HTTP Server

```python
# agents/web-server.py.socket
# ENABLED = True
# TYPE = socket
# LISTEN_STREAM = 0.0.0.0:8000

import os
import socket
from http.server import HTTPServer, BaseHTTPRequestHandler

class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200)
        self.send_header('Content-type', 'text/html')
        self.end_headers()
        self.wfile.write(b"Hello from GAPI!")

def start():
    fd = int(os.environ.get("LISTEN_FDS", "0"))
    if fd == 0:
        return
    
    # Create HTTP server with existing socket
    sock = socket.fromfd(fd + 3, socket.AF_INET, socket.SOCK_STREAM)
    server = HTTPServer(('0.0.0.0', 8000), Handler)
    server.socket = sock
    server.serve_forever()
```

## Resource-Limited Agents

Agents with CPU and memory constraints.

### CPU-Limited Worker

```python
# agents/cpu-worker.py.service
ENABLED = True
TYPE = "service"
CPU_LIMIT = 0.5

def start():
    # This agent can use at most 50% of one CPU core
    while True:
        # CPU-intensive work
        pass
```

### Memory-Limited Cache

```python
# agents/cache.py.service
ENABLED = True
TYPE = "service"
MEMORY_LIMIT = "512MB"

def start():
    # This agent will be OOM-killed if it exceeds 512MB
    cache = {}
    # Implement cache with memory awareness
```

### Combined Limits

```python
# agents/batch-processor.py.service
# ENABLED = True
# TYPE = service
# CPU_LIMIT = 2.0
# MEMORY_LIMIT = 2GB

def start():
    # Can use up to 2 CPU cores and 2GB of memory
    # Process large batches
```

## Real-World Use Cases

### Database Backup Timer

```python
# agents/db-backup.py.timer
ENABLED = True
TYPE = "timer"
SCHEDULE = "0 2 * * *"
DEPENDENCIES = ["database"]

import subprocess
import datetime

def start():
    timestamp = datetime.datetime.now().strftime("%Y%m%d_%H%M%S")
    backup_file = f"/backups/db_{timestamp}.sql"
    
    subprocess.run([
        "pg_dump",
        "-h", "localhost",
        "-U", "postgres",
        "-f", backup_file,
        "mydb"
    ])
    
    print(f"Backup created: {backup_file}")
```

### Log Rotation Service

```python
# agents/log-rotator.py.timer
ENABLED = True
TYPE = "timer"
SCHEDULE = "@daily"

import os
import gzip
import shutil
from datetime import datetime

def start():
    log_dir = "/var/log/gapi"
    
    for filename in os.listdir(log_dir):
        if filename.endswith(".log"):
            log_path = os.path.join(log_dir, filename)
            
            # Compress and rotate
            timestamp = datetime.now().strftime("%Y%m%d")
            archive_path = f"{log_path}.{timestamp}.gz"
            
            with open(log_path, 'rb') as f_in:
                with gzip.open(archive_path, 'wb') as f_out:
                    shutil.copyfileobj(f_in, f_out)
            
            # Clear original log
            open(log_path, 'w').close()
            
            print(f"Rotated: {filename}")
```

### Metrics Collector

```python
# agents/metrics.py.timer
# ENABLED = True
# TYPE = timer
# SCHEDULE = 1m

import psutil
import json
from gapi import eventbus

def start():
    metrics = {
        "cpu_percent": psutil.cpu_percent(),
        "memory_percent": psutil.virtual_memory().percent,
        "disk_percent": psutil.disk_usage('/').percent,
    }
    
    # Publish to event bus
    eventbus.publish("metrics", "system.stats", metrics)
    
    print(f"Metrics: {json.dumps(metrics)}")
```

### API Gateway with Rate Limiting

```python
# agents/gateway.py.socket
# ENABLED = True
# TYPE = socket
# LISTEN_STREAM = 0.0.0.0:8080
# CPU_LIMIT = 1.0
# MEMORY_LIMIT = 512MB

import os
import socket
import time
from collections import defaultdict

rate_limits = defaultdict(list)

def start():
    fd = int(os.environ.get("LISTEN_FDS", "0"))
    sock = socket.fromfd(fd + 3, socket.AF_INET, socket.SOCK_STREAM)
    
    while True:
        conn, addr = sock.accept()
        
        # Rate limiting
        now = time.time()
        client_ip = addr[0]
        
        # Remove old requests
        rate_limits[client_ip] = [t for t in rate_limits[client_ip] if now - t < 60]
        
        if len(rate_limits[client_ip]) >= 100:
            conn.sendall(b"HTTP/1.1 429 Too Many Requests\r\n\r\n")
            conn.close()
            continue
        
        rate_limits[client_ip].append(now)
        
        # Handle request
        data = conn.recv(1024)
        conn.sendall(b"HTTP/1.1 200 OK\r\n\r\nOK")
        conn.close()
```

### Health Check Monitor

```python
# agents/health-check.py.timer
# ENABLED = True
# TYPE = timer
# SCHEDULE = 30s

import requests
from gapi import eventbus

ENDPOINTS = [
    "http://localhost:8080/health",
    "http://localhost:8081/health",
]

def start():
    for endpoint in ENDPOINTS:
        try:
            resp = requests.get(endpoint, timeout=5)
            status = "healthy" if resp.status_code == 200 else "unhealthy"
        except Exception as e:
            status = "down"
        
        eventbus.publish("health", f"check.{endpoint}", {"status": status})
        print(f"{endpoint}: {status}")
```
