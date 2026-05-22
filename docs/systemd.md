# Running under systemd

Reference unit + service-user setup. Copy what you need; nothing here is mandatory — the binary runs fine in the foreground for testing.

## Service user

A dedicated, no-shell, no-home system account.

```sh
sudo useradd --system --no-create-home --shell /usr/sbin/nologin intel-gpu-exporter
```

The unit below uses `SupplementaryGroups=render video` to grant `/dev/dri/*` access — scoped to this service, no need to add the system user to those groups globally. The `render` group owns the DRM render nodes on every modern distro (gid varies — 226 on Debian/Ubuntu, 39 on Arch, etc.); using the name avoids hard-coding the number.

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
SupplementaryGroups=render video
# PMU access via perf_event_open needs CAP_PERFMON on most kernels.
# Drop this line (and CapabilityBoundingSet below) if --collector.pmu=false.
AmbientCapabilities=CAP_PERFMON
CapabilityBoundingSet=CAP_PERFMON
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

If you're not running the PMU collector (`--collector.pmu=false`), drop the `AmbientCapabilities=` and `CapabilityBoundingSet=` lines entirely. The exporter will degrade to sysfs + hwmon + fdinfo + Level Zero, all of which work fine with just the `render` group membership.

`CAP_SYS_ADMIN` was previously listed here too — it isn't actually required for any collector and has been removed. Add it back only if a future kernel/driver combination demands it.

## Removing

```sh
sudo systemctl disable --now intel_gpu_exporter
sudo rm /etc/systemd/system/intel_gpu_exporter.service
sudo rm /usr/local/bin/intel_gpu_exporter
sudo userdel intel-gpu-exporter
```

(no `usermod -G` to undo — the group membership was scoped to the unit via `SupplementaryGroups=`, not added to the user.)
