#!/bin/bash
set -euxo pipefail

REPO=arnaudmorin/fireworks
BIN=/usr/local/bin/fireworks
USER_NAME=fireworks

case "$(uname -m)" in
  x86_64) ARCH=amd64 ;;
  aarch64) ARCH=arm64 ;;
  *) echo "unsupported architecture: $(uname -m)" >&2; exit 1 ;;
esac

tmp=$(mktemp -d)
base="https://github.com/$REPO/releases/latest/download"
curl -fsSL --retry 10 --retry-all-errors -o "$tmp/fireworks-linux-$ARCH" "$base/fireworks-linux-$ARCH"
curl -fsSL --retry 10 --retry-all-errors -o "$tmp/SHA256SUMS" "$base/SHA256SUMS"
(cd "$tmp" && sha256sum --check --ignore-missing SHA256SUMS)
install -m 0755 "$tmp/fireworks-linux-$ARCH" "$BIN"
rm -rf "$tmp"
grep -qx "$BIN" /etc/shells || echo "$BIN" >> /etc/shells

id "$USER_NAME" >/dev/null 2>&1 || useradd --create-home --shell "$BIN" --comment "Feu d'artifice" "$USER_NAME"
passwd -d "$USER_NAME"
touch "/home/$USER_NAME/.hushlogin"
chown -R root:root "/home/$USER_NAME"
chmod 0755 "/home/$USER_NAME"

# sshd's "none" auth with an empty password goes through PAM, which needs nullok
grep -Eq '^auth.*pam_unix\.so.*nullok' /etc/pam.d/common-auth \
  || sed -i -E '/^auth.*pam_unix\.so/ s/$/ nullok/' /etc/pam.d/common-auth

uid=$(id -u "$USER_NAME")
mkdir -p "/etc/systemd/system/user-$uid.slice.d"
printf '[Slice]\nCPUQuota=200%%\nMemoryMax=1G\nTasksMax=2048\n' > "/etc/systemd/system/user-$uid.slice.d/fireworks.conf"
systemctl daemon-reload

MESSAGE=$(curl -fsS --retry 5 --retry-all-errors http://169.254.169.254/openstack/latest/meta_data.json \
  | python3 -c 'import json, sys; print(json.load(sys.stdin).get("meta", {}).get("fireworks_message", ""))' \
  || true)
setenv=""
if [ -n "$MESSAGE" ]; then
  esc=$(printf '%s' "$MESSAGE" | tr '\r\n' '  ')
  esc=${esc//\\/\\\\}
  esc=${esc//\"/\\\"}
  setenv="    SetEnv \"FIREWORKS_MESSAGE=$esc\""
fi

cat > /etc/ssh/sshd_config.d/00-fireworks.conf <<EOF
Match User $USER_NAME
$setenv
    PasswordAuthentication yes
    PermitEmptyPasswords yes
    PubkeyAuthentication no
    ForceCommand $BIN
    DisableForwarding yes
    PermitTunnel no
    PermitUserRC no
    PermitTTY yes
    MaxSessions 1
    ClientAliveInterval 30
    ClientAliveCountMax 3
EOF

mkdir -p /run/sshd
if ! { sshd -t && sshd -T -C "user=$USER_NAME,host=localhost,addr=127.0.0.1" >/dev/null; }; then
  rm -f /etc/ssh/sshd_config.d/00-fireworks.conf
  echo "sshd rejected the fireworks config, anonymous access NOT enabled" >&2
  exit 1
fi
systemctl try-reload-or-restart ssh
