# Running under systemd

Reference unit + service-user setup. Copy what you need; nothing here is mandatory — the binary runs fine in the foreground for testing.

## Service user

A dedicated, no-shell, no-home system account. The `render` and `video` group memberships let it open `/dev/dri/*` without granting root.

```sh
sudo useradd --system --no-create-home --shell /usr/sbin/nologin intel-gpu-exporter
sudo usermod -a -G render,video intel-gpu-exporter
```

## Unit file

Drop this at `/etc/systemd/system/intel_gpu_exporter.service`, adjusting `ExecStart=` flags to taste.

```ini
[Unit]
Description=Prometheus exporter for Intel GPU metrics
Documentation=https://github.com/sratabix/intel_gpu_exporter
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=intel-gpu-exporter
Group=intel-gpu-exporter
# intel_gpu_top requires CAP_PERFMON (or root) for PMU access on most kernels.
# Adjust the line below depending on which collectors you enable.
AmbientCapabilities=CAP_PERFMON CAP_SYS_ADMIN
CapabilityBoundingSet=CAP_PERFMON CAP_SYS_ADMIN
ExecStart=/usr/local/bin/intel_gpu_exporter \
    --web.listen-address=:9404 \
    --log.level=info
Restart=on-failure
RestartSec=5

# Hardening
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
PrivateDevices=false
DeviceAllow=/dev/dri rw
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
LockPersonality=true
MemoryDenyWriteExecute=true
RestrictRealtime=true
RestrictNamespaces=true
SystemCallFilter=@system-service
SystemCallErrorNumber=EPERM
ReadOnlyPaths=/sys /proc

[Install]
WantedBy=multi-user.target
```

Then:

```sh
sudo systemctl daemon-reload
sudo systemctl enable --now intel_gpu_exporter
sudo journalctl -fu intel_gpu_exporter
```

`curl http://localhost:9404/metrics` should now return Prometheus exposition.

## Trimming capabilities

If you're not running the PMU collector (`--collector.pmu=false`), drop `CAP_PERFMON` and `CAP_SYS_ADMIN` from both `AmbientCapabilities` and `CapabilityBoundingSet` lines. The exporter will degrade to sysfs + hwmon + fdinfo + Level Zero, all of which work without elevated capabilities.

## Removing

```sh
sudo systemctl disable --now intel_gpu_exporter
sudo rm /etc/systemd/system/intel_gpu_exporter.service
sudo rm /usr/local/bin/intel_gpu_exporter
sudo userdel intel-gpu-exporter
```
