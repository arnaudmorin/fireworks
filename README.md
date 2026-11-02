# fireworks

Terminal fireworks, meant to be served as the login shell of an anonymous SSH account:

```
ssh fireworks@<ip>
```

A 10-second show over a night skyline, ending with a finale and a message written in sparks.

## Run locally

```
go build -o fireworks . && ./fireworks
```

Keys: `space` salvo, `f` finale, `c` toggle truecolor / 256 colors, `q` quit.

A local build runs until you quit. The session length is set at build time:

```
go build -ldflags "-X main.maxDuration=10s" -o fireworks .
```

The finale ends with a message written in sparks (default `Merci Emma!`). Change it with, by priority:

```
./fireworks -message "Joyeux anniversaire ♥"      # flag (also --message=..., -m)
FIREWORKS_MESSAGE="Bravo !" ./fireworks           # environment, empty disables the message
go build -ldflags "-X 'main.message=Hello'" .     # build-time default
```

Letters, digits, common punctuation and `♥` are supported; accents are folded (É → E), long messages wrap.

Pure standard library, Linux only (raw terminal handling via `ioctl`).

## Releases

Every push to `master` triggers [`.github/workflows/release.yml`](.github/workflows/release.yml), which builds static
`linux/amd64` and `linux/arm64` binaries (10 s sessions) and publishes them as the latest GitHub release, together with
a `SHA256SUMS` file.

## Deploy on OpenStack

[`userdata.sh`](userdata.sh) is a plain cloud-init user-data script for Ubuntu/Debian images. It:

- downloads the latest release binary and verifies its checksum,
- creates a `fireworks` user with an empty password and the binary as login shell,
- adds an sshd `Match User fireworks` block: empty password only, `ForceCommand`, no forwarding/tunnels/sftp,
- reads the optional `fireworks_message` instance property and passes it to the binary (sshd `SetEnv`),
- caps all fireworks sessions together at 2 CPUs / 1 GB RAM through the user's systemd slice.

Other accounts are untouched: the image's default user keeps key-only access for administration.

With the OpenStack CLI:

```
openstack server create --image "Ubuntu 24.04" --flavor d2-2 --network Ext-Net \
  --key-name <key> --security-group <sg allowing tcp/22> \
  --property fireworks_message="Merci Emma!" \
  --user-data userdata.sh fireworks
```

With OpenTofu (credentials from your `openrc`):

```
source openrc.sh
cd tofu
tofu init
tofu apply -var message="Merci Emma!"   # -var flavor_name=... -var network_name=... as needed
tofu output -raw ssh_command
```

Allow a couple of minutes after boot for the setup to finish (`/var/log/cloud-init-output.log`).

## Security notes

Anyone who can reach port 22 can log in as `fireworks`. The account only ever runs the fireworks binary: no shell,
no port forwarding, no file transfer, and the session ends after 10 seconds. Don't reuse the instance for anything else.

## License

[Apache 2.0](LICENSE)
