#!/bin/bash
set -euo pipefail
# Install independent units; preserve the existing Umbrel/firewall pre-start hook.
root_dir="$(cd "$(dirname "$0")" && pwd)"
install -d -m 700 "$root_dir/systemd"
cat > "$root_dir/systemd/viki-vozokany-backup.service" <<UNIT
[Unit]
Description=Consistent backup of the separate Vozokany Viki instance
After=docker.service
Requires=docker.service

[Service]
Type=oneshot
WorkingDirectory=$root_dir
ExecStart=$root_dir/backup.sh
Nice=10
IOSchedulingClass=idle
TimeoutStartSec=15min
UNIT
cat > "$root_dir/systemd/viki-vozokany-backup.timer" <<'UNIT'
[Unit]
Description=Daily separate Vozokany Viki backup

[Timer]
OnCalendar=*-*-* 03:30:00 Europe/Bratislava
RandomizedDelaySec=10m
Persistent=true
Unit=viki-vozokany-backup.service

[Install]
WantedBy=timers.target
UNIT
ln -sfn "$root_dir/systemd/viki-vozokany-backup.service" /etc/systemd/system/viki-vozokany-backup.service
ln -sfn "$root_dir/systemd/viki-vozokany-backup.timer" /etc/systemd/system/viki-vozokany-backup.timer
systemctl daemon-reload
systemctl enable --now viki-vozokany-backup.timer
